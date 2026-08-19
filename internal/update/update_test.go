package update

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	chaisync "github.com/charliesbot/chai/internal/sync"
)

func TestRunWithHomeRefreshesGitHubSourcesThenSyncs(t *testing.T) {
	cfg := &config.Config{
		Platforms: []string{"cursor"},
		Skills: config.Skills{GitHub: []config.GitHubSkills{
			{URL: "https://github.com/example/one", Include: []string{"chosen-one"}},
			{URL: "https://github.com/example/two", Include: []string{"chosen-two"}},
		}},
	}
	var mu sync.Mutex
	refreshed := make(map[string]bool)
	var synced bool
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Refresh: func(_ context.Context, _ string, id githubskill.Identity, names []string) (githubskill.Discovery, error) {
			mu.Lock()
			refreshed[id.URL()+":"+names[0]] = true
			mu.Unlock()
			return githubskill.Discovery{Candidates: []githubskill.Candidate{{Name: names[0]}, {Name: "new-skill"}}}, nil
		},
		Sync: func(_ context.Context, got *config.Config, _ string, _ chaisync.Options) error {
			mu.Lock()
			defer mu.Unlock()
			synced = len(refreshed) == 2 && got == cfg
			return nil
		},
	}

	if err := RunWithHome(context.Background(), cfg, t.TempDir(), opts); err != nil {
		t.Fatal(err)
	}
	wantRefreshed := map[string]bool{
		"https://github.com/example/one:chosen-one": true,
		"https://github.com/example/two:chosen-two": true,
	}
	if !reflect.DeepEqual(refreshed, wantRefreshed) || !synced {
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

func TestRunWithHomeRejectsLocalOnlyNameConflicts(t *testing.T) {
	home := t.TempDir()
	collection := filepath.Join(home, "locals")
	writeUpdateSkill(t, filepath.Join(collection, "one"), "shared")
	writeUpdateSkill(t, filepath.Join(collection, "two"), "shared")
	cfg := &config.Config{
		Platforms: []string{"cursor"},
		Skills:    config.Skills{Local: []string{collection}},
	}

	err := RunWithHome(context.Background(), cfg, home, Options{})
	if err == nil || !strings.Contains(err.Error(), filepath.Join(collection, "one")) || !strings.Contains(err.Error(), filepath.Join(collection, "two")) {
		t.Fatalf("local conflict error = %v", err)
	}
}

func TestValidateSourceNamesReportsEveryConflictLocation(t *testing.T) {
	home := t.TempDir()
	collection := filepath.Join(home, "locals")
	writeUpdateSkill(t, filepath.Join(collection, "one"), "one")
	writeUpdateSkill(t, filepath.Join(collection, "two"), "two")
	writeUpdateSkill(t, filepath.Join(collection, "shared-a"), "shared")
	writeUpdateSkill(t, filepath.Join(collection, "shared-b"), "shared")
	cfg := &config.Config{
		Platforms: []string{"cursor"},
		Skills: config.Skills{
			Local: []string{collection},
			GitHub: []config.GitHubSkills{
				{URL: "https://github.com/example/one", Include: []string{"one"}},
				{URL: "https://github.com/example/two", Include: []string{"two"}},
				{URL: "https://github.com/example/shared", Include: []string{"shared"}},
			},
		},
	}

	err := validateSourceNames(cfg, home)
	if err == nil {
		t.Fatal("expected name conflicts")
	}
	for _, detail := range []string{
		filepath.Join(collection, "one"),
		filepath.Join(collection, "two"),
		filepath.Join(collection, "shared-a"),
		filepath.Join(collection, "shared-b"),
		"https://github.com/example/one",
		"https://github.com/example/two",
		"https://github.com/example/shared",
	} {
		if !strings.Contains(err.Error(), detail) {
			t.Errorf("error %q does not contain %q", err, detail)
		}
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

func writeUpdateSkill(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
}
