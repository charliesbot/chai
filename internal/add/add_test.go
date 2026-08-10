package add

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	"github.com/charliesbot/chai/internal/skill"
	chaisync "github.com/charliesbot/chai/internal/sync"
)

func TestParseArgsConsumesSkillNamesUntilNextOption(t *testing.T) {
	request, err := ParseArgs([]string{"owner/repo", "--skill", "one", "two", "--yes", "--global"})
	if err != nil {
		t.Fatal(err)
	}
	if request.Source != "owner/repo" || !reflect.DeepEqual(request.Skills, []string{"one", "two"}) {
		t.Fatalf("request = %#v", request)
	}
}

func TestNormalizeLocalPath(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "Users", "test")
	manifestDir := filepath.Join(home, "dotfiles")
	cases := map[string]string{
		filepath.Join(home, "skills") + string(filepath.Separator): "~/skills",
		"~/skills/../skills": "~/skills",
		"./skills/../skills": "./skills",
		"./skills/..":        "./.",
		"../shared/skills/":  "../shared/skills",
		"../skills/..":       "../.",
	}
	for input, want := range cases {
		if got, err := NormalizeLocalPath(input, manifestDir, home); err != nil || got != want {
			t.Fatalf("NormalizeLocalPath(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestRunWithHomeAddsLocalSourceAndSyncs(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	root := filepath.Join(home, "skills")
	writeSkill(t, root, "local-skill")
	cfg := &config.Config{Platforms: []string{"cursor"}}
	synced := false
	opts := Options{
		Sync: func(context.Context, *config.Config, string, chaisync.Options) error {
			synced = true
			return nil
		},
	}
	if err := RunWithHome(context.Background(), cfg, manifest, home, []string{root}, opts); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !synced || !reflect.DeepEqual(loaded.Skills.Local, []string{"~/skills"}) {
		t.Fatalf("synced=%v local=%v", synced, loaded.Skills.Local)
	}
}

func TestRunWithHomeLocalAddRunsWithoutConfirmationSummary(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	root := filepath.Join(home, "skills")
	writeSkill(t, filepath.Join(root, "two"), "two")
	writeSkill(t, filepath.Join(root, "one"), "one")
	cfg := &config.Config{Platforms: []string{"cursor", "claude"}}
	var output bytes.Buffer
	opts := Options{
		Sync:   func(context.Context, *config.Config, string, chaisync.Options) error { return nil },
		Output: &output,
	}

	if err := RunWithHome(context.Background(), cfg, manifest, home, []string{root}, opts); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Errorf("local add should only show sync results, got %q", output.String())
	}
}

func TestRunWithHomeAddsSelectedRemoteAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	cfg := &config.Config{Platforms: []string{"cursor"}}
	source := testRepository(t)
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	syncs := 0
	var output bytes.Buffer
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Discover: func(ctx context.Context, _ githubskill.Identity, repository string) (githubskill.Discovery, error) {
			return githubskill.Discover(ctx, cloneURL, repository)
		},
		Sync: func(context.Context, *config.Config, string, chaisync.Options) error {
			syncs++
			return nil
		},
		Output: &output,
	}
	firstArgs := []string{"example/skills", "--skill", "one", "--yes"}
	if err := RunWithHome(context.Background(), cfg, manifest, home, firstArgs, opts); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(manifest)
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	mergeArgs := []string{"example/skills", "--skill", "two", "--yes"}
	if err := RunWithHome(context.Background(), loaded, manifest, home, mergeArgs, opts); err != nil {
		t.Fatal(err)
	}
	loaded, err = config.Load(manifest)
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunWithHome(context.Background(), loaded, manifest, home, mergeArgs, opts); err != nil {
		t.Fatal(err)
	}
	loaded, err = config.Load(manifest)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"one", "two"}
	if syncs != 3 || len(loaded.Skills.GitHub) != 1 || !reflect.DeepEqual(loaded.Skills.GitHub[0].Include, want) {
		t.Fatalf("syncs=%d github=%#v", syncs, loaded.Skills.GitHub)
	}
}

func TestRunWithHomeRemoteAddReportsProgressWithoutConfirmationSummary(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	cfg := &config.Config{Platforms: []string{"claude", "cursor"}}
	source := testRepository(t)
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	var labels []string
	synced := false
	var output bytes.Buffer
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Discover: func(ctx context.Context, _ githubskill.Identity, repository string) (githubskill.Discovery, error) {
			return githubskill.Discover(ctx, cloneURL, repository)
		},
		Progress: func(label string, operation func() error) error {
			labels = append(labels, label)
			return operation()
		},
		Sync: func(context.Context, *config.Config, string, chaisync.Options) error {
			synced = true
			return nil
		},
		Output: &output,
	}

	if err := RunWithHome(context.Background(), cfg, manifest, home, []string{"example/skills", "--skill", "one"}, opts); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(labels, []string{"Inspecting example/skills", "Fetching 1 selected skill(s)"}) {
		t.Fatalf("progress labels = %v", labels)
	}
	if !synced {
		t.Fatal("remote add did not continue directly to sync")
	}
	for _, unwanted := range []string{"GitHub source", "Selected skills", "Manifest", "Platforms", "Continue?"} {
		if strings.Contains(output.String(), unwanted) {
			t.Errorf("add output contains confirmation summary %q:\n%s", unwanted, output.String())
		}
	}
}

func TestRunWithHomeRemoteAddRejectsUnmanagedDestinationsBeforeMutation(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	cfg := &config.Config{Platforms: []string{"cursor"}}
	writeSkill(t, filepath.Join(home, ".cursor", "skills", "one"), "one")
	source := testRepository(t)
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Discover: func(ctx context.Context, _ githubskill.Identity, repository string) (githubskill.Discovery, error) {
			return githubskill.Discover(ctx, cloneURL, repository)
		},
		Progress: func(_ string, operation func() error) error { return operation() },
	}

	err := RunWithHome(context.Background(), cfg, manifest, home, []string{"example/skills", "--skill", "one"}, opts)
	if err == nil || !strings.Contains(err.Error(), filepath.Join(home, ".cursor", "skills", "one")) {
		t.Fatalf("collision error = %v", err)
	}
	if _, statErr := os.Stat(manifest); !os.IsNotExist(statErr) {
		t.Fatalf("manifest changed before destination validation: %v", statErr)
	}
	id, _ := githubskill.ParseCanonical("https://github.com/example/skills")
	if _, statErr := os.Stat(githubskill.CacheDir(home, id)); !os.IsNotExist(statErr) {
		t.Fatalf("cache promoted before destination validation: %v", statErr)
	}
}

func TestRejectNameConflictsReportsEveryLocation(t *testing.T) {
	selected := []skill.Source{{Name: "one", Path: "new-source"}, {Name: "two", Path: "new-source"}}
	locals := []skill.Source{{Name: "one", Path: "local-one"}}
	cfg := &config.Config{Skills: config.Skills{GitHub: []config.GitHubSkills{
		{URL: "https://github.com/example/existing", Include: []string{"two"}},
	}}}

	err := rejectNameConflicts(selected, locals, cfg, "")
	if err == nil {
		t.Fatal("expected name conflicts")
	}
	for _, detail := range []string{`"one": local-one, new-source`, `"two": https://github.com/example/existing, new-source`} {
		if !strings.Contains(err.Error(), detail) {
			t.Errorf("error %q does not contain %q", err, detail)
		}
	}
}

func TestRunWithHomeRemoteAddCompletesRealSync(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	cfg := &config.Config{Platforms: []string{"cursor"}}
	source := testRepository(t)
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Discover: func(ctx context.Context, _ githubskill.Identity, repository string) (githubskill.Discovery, error) {
			return githubskill.Discover(ctx, cloneURL, repository)
		},
	}
	if err := RunWithHome(context.Background(), cfg, manifest, home, []string{"example/skills", "--skill", "one", "--yes"}, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "skills", "one", "SKILL.md")); err != nil {
		t.Fatalf("platform skill missing after add sync: %v", err)
	}
	loaded, err := config.Load(manifest)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := githubskill.ParseCanonical(loaded.Skills.GitHub[0].URL)
	if _, err := githubskill.ResolveCached(home, id, []string{"one"}); err != nil {
		t.Fatalf("promoted cache is incomplete: %v", err)
	}
}

func TestRunWithHomeRejectsConflictBeforePromotion(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	local := filepath.Join(home, "local")
	writeSkill(t, local, "one")
	cfg := &config.Config{Platforms: []string{"cursor"}, Skills: config.Skills{Local: []string{local}}}
	source := testRepository(t)
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Discover: func(ctx context.Context, _ githubskill.Identity, repository string) (githubskill.Discovery, error) {
			return githubskill.Discover(ctx, cloneURL, repository)
		},
	}
	err := RunWithHome(context.Background(), cfg, manifest, home, []string{"example/skills", "--skill", "one"}, opts)
	if err == nil {
		t.Fatal("expected name conflict")
	}
	id, _ := githubskill.ParseCanonical("https://github.com/example/skills")
	if _, statErr := os.Stat(githubskill.CacheDir(home, id)); !os.IsNotExist(statErr) {
		t.Fatalf("cache promoted before validation: %v", statErr)
	}
}

func TestRunWithHomeRestoresExistingCacheWhenManifestWriteFails(t *testing.T) {
	home := t.TempDir()
	id, _ := githubskill.ParseCanonical("https://github.com/example/skills")
	cache := githubskill.CacheDir(home, id)
	oldSkill := filepath.Join(githubskill.RepositoryDir(cache), "old-one")
	if err := os.MkdirAll(oldSkill, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldSkill, "SKILL.md"), []byte("---\nname: one\n---\nold cache\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := githubskill.CompleteStaging(cache, id, map[string]string{"one": "old-one"}, "old-commit"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Platforms: []string{"cursor"},
		Skills: config.Skills{GitHub: []config.GitHubSkills{{
			URL: id.URL(), Include: []string{"one"},
		}}},
	}
	source := testRepository(t)
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	configPath := filepath.Join(home, "manifest-is-a-directory")
	if err := os.Mkdir(configPath, 0755); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Discover: func(ctx context.Context, _ githubskill.Identity, repository string) (githubskill.Discovery, error) {
			return githubskill.Discover(ctx, cloneURL, repository)
		},
	}
	if err := RunWithHome(context.Background(), cfg, configPath, home, []string{"example/skills", "--skill", "one", "--yes"}, opts); err == nil {
		t.Fatal("expected manifest write failure")
	}
	sources, err := githubskill.ResolveCached(home, id, []string{"one"})
	if err != nil {
		t.Fatalf("previous cache was not restored: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(sources[0].Path, "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "old cache") {
		t.Fatalf("restored content = %q, err=%v", data, err)
	}
}

func TestRunWithHomeSyncsWhenPreviousCacheCleanupFails(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	cfg := &config.Config{Platforms: []string{"cursor"}}
	source := testRepository(t)
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	synced := false
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Discover: func(ctx context.Context, _ githubskill.Identity, repository string) (githubskill.Discovery, error) {
			return githubskill.Discover(ctx, cloneURL, repository)
		},
		CommitPromotion: func(githubskill.Promotion) error { return assertError("cleanup failed") },
		Sync: func(context.Context, *config.Config, string, chaisync.Options) error {
			synced = true
			return nil
		},
	}
	err := RunWithHome(context.Background(), cfg, manifest, home, []string{"example/skills", "--skill", "one", "--yes"}, opts)
	if err == nil || !strings.Contains(err.Error(), "recorded and synced") {
		t.Fatalf("cleanup error = %v", err)
	}
	if !synced {
		t.Fatal("sync did not run after cache cleanup failure")
	}
}

func TestRunWithHomeMalformedCandidatePolicy(t *testing.T) {
	source := testRepository(t)
	broken := filepath.Join(source, "broken", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(broken), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken, []byte("missing frontmatter"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, source, "add", ".")
	runGit(t, source, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "broken candidate")
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	discover := func(ctx context.Context, _ githubskill.Identity, repository string) (githubskill.Discovery, error) {
		return githubskill.Discover(ctx, cloneURL, repository)
	}

	explicitHome := t.TempDir()
	explicitManifest := filepath.Join(explicitHome, "chai.toml")
	explicitCfg := &config.Config{Platforms: []string{"cursor"}}
	if err := RunWithHome(context.Background(), explicitCfg, explicitManifest, explicitHome,
		[]string{"example/skills", "--skill", "one", "--yes"},
		Options{CheckGit: func(context.Context) error { return nil }, Discover: discover, Sync: func(context.Context, *config.Config, string, chaisync.Options) error { return nil }}); err != nil {
		t.Fatalf("explicit selection should ignore unrelated malformed candidate: %v", err)
	}

	allHome := t.TempDir()
	allManifest := filepath.Join(allHome, "chai.toml")
	allCfg := &config.Config{Platforms: []string{"cursor"}}
	err := RunWithHome(context.Background(), allCfg, allManifest, allHome,
		[]string{"example/skills", "--yes"},
		Options{CheckGit: func(context.Context) error { return nil }, Discover: discover})
	if err == nil || !strings.Contains(err.Error(), "cannot add all") {
		t.Fatalf("add-all malformed candidate error = %v", err)
	}
	if _, err := os.Stat(allManifest); !os.IsNotExist(err) {
		t.Fatalf("add-all changed manifest: %v", err)
	}
	id, _ := githubskill.ParseCanonical("https://github.com/example/skills")
	if _, err := os.Stat(githubskill.CacheDir(allHome, id)); !os.IsNotExist(err) {
		t.Fatalf("add-all promoted cache: %v", err)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

func writeSkill(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func testRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	runGit(t, repository, "init", "-b", "main")
	runGit(t, repository, "config", "uploadpack.allowFilter", "true")
	writeSkill(t, filepath.Join(repository, "skills", "one"), "one")
	writeSkill(t, filepath.Join(repository, "plugin", "two"), "two")
	runGit(t, repository, "add", ".")
	runGit(t, repository, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "skills")
	return repository
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s: %v", args, output, err)
	}
}
