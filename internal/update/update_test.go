package update

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	chaisync "github.com/charliesbot/chai/internal/sync"
)

func TestRunWithHomeRefreshesGitHubSourcesThenSyncs(t *testing.T) {
	cfg := &config.Config{
		Platforms: []string{"cursor"},
		Skills: config.Skills{GitHub: []config.GitHubSkills{{
			URL: "https://github.com/example/skills", Include: []string{"chosen"},
		}}},
	}
	var refreshed, synced bool
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Refresh: func(_ context.Context, _ string, id githubskill.Identity, names []string) (githubskill.Discovery, error) {
			refreshed = id.URL() == cfg.Skills.GitHub[0].URL && len(names) == 1 && names[0] == "chosen"
			return githubskill.Discovery{Candidates: []githubskill.Candidate{{Name: "chosen"}, {Name: "new-skill"}}}, nil
		},
		Sync: func(_ context.Context, got *config.Config, _ string, _ chaisync.Options) error {
			synced = refreshed && got == cfg
			return nil
		},
	}

	if err := RunWithHome(context.Background(), cfg, t.TempDir(), opts); err != nil {
		t.Fatal(err)
	}
	if !refreshed || !synced {
		t.Fatalf("refreshed=%v synced=%v", refreshed, synced)
	}
}

func TestRunWithHomeChecksGitBeforeRefresh(t *testing.T) {
	cfg := &config.Config{
		Platforms: []string{"cursor"},
		Skills: config.Skills{GitHub: []config.GitHubSkills{{
			URL: "https://github.com/example/skills", Include: []string{"chosen"},
		}}},
	}
	called := false
	opts := Options{
		CheckGit: func(context.Context) error { return assertError("old git") },
		Refresh: func(context.Context, string, githubskill.Identity, []string) (githubskill.Discovery, error) {
			called = true
			return githubskill.Discovery{}, nil
		},
	}
	if err := RunWithHome(context.Background(), cfg, t.TempDir(), opts); err == nil {
		t.Fatal("expected Git error")
	}
	if called {
		t.Fatal("refresh ran before Git preflight succeeded")
	}
}

func TestRunWithHomeRejectsNameConflictBeforeGitOrCacheMutation(t *testing.T) {
	home := t.TempDir()
	local := filepath.Join(home, "local")
	if err := os.MkdirAll(local, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "SKILL.md"), []byte("---\nname: chosen\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Platforms: []string{"cursor"},
		Skills: config.Skills{
			Local:  []string{local},
			GitHub: []config.GitHubSkills{{URL: "https://github.com/example/skills", Include: []string{"chosen"}}},
		},
	}
	called := false
	opts := Options{
		CheckGit: func(context.Context) error {
			called = true
			return nil
		},
	}
	err := RunWithHome(context.Background(), cfg, home, opts)
	if err == nil {
		t.Fatal("expected name conflict")
	}
	if called {
		t.Fatal("Git preflight ran before source conflict validation")
	}
}

func TestAvailableSkillsSkipsAmbiguousNames(t *testing.T) {
	source := config.GitHubSkills{Include: []string{"selected"}}
	discovery := githubskill.Discovery{Candidates: []githubskill.Candidate{
		{Name: "selected"},
		{Name: "available"},
		{Name: "ambiguous"},
		{Name: "ambiguous"},
	}}
	if got := availableSkills(source, discovery); !reflect.DeepEqual(got, []string{"available"}) {
		t.Fatalf("available skills = %v", got)
	}
}

func TestRunWithHomeSyncsAfterCacheCleanupWarning(t *testing.T) {
	cfg := &config.Config{
		Platforms: []string{"cursor"},
		Skills: config.Skills{GitHub: []config.GitHubSkills{{
			URL: "https://github.com/example/skills", Include: []string{"chosen"},
		}}},
	}
	synced := false
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Refresh: func(context.Context, string, githubskill.Identity, []string) (githubskill.Discovery, error) {
			return githubskill.Discovery{}, &githubskill.CleanupError{Err: assertError("cleanup failed")}
		},
		Sync: func(context.Context, *config.Config, string, chaisync.Options) error {
			synced = true
			return nil
		},
	}
	if err := RunWithHome(context.Background(), cfg, t.TempDir(), opts); err != nil {
		t.Fatal(err)
	}
	if !synced {
		t.Fatal("sync did not run after cleanup-only refresh error")
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }
