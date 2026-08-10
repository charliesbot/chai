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
	if request.Source != "owner/repo" || !reflect.DeepEqual(request.Skills, []string{"one", "two"}) || !request.Yes {
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
		Confirm: func(string) (bool, error) { return true, nil },
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

func TestRunWithHomeLocalSummaryShowsTrackedSkillsAndManifestDelta(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	root := filepath.Join(home, "skills")
	writeSkill(t, filepath.Join(root, "two"), "two")
	writeSkill(t, filepath.Join(root, "one"), "one")
	cfg := &config.Config{Platforms: []string{"cursor", "claude"}}
	var output bytes.Buffer
	opts := Options{
		Confirm: func(string) (bool, error) { return true, nil },
		Sync:    func(context.Context, *config.Config, string, chaisync.Options) error { return nil },
		Output:  &output,
	}

	if err := RunWithHome(context.Background(), cfg, manifest, home, []string{root}, opts); err != nil {
		t.Fatal(err)
	}
	for _, detail := range []string{"~/skills", "tracked skills: one, two", "manifest: add local source", "sync to cursor, claude"} {
		if !strings.Contains(output.String(), detail) {
			t.Errorf("summary %q does not contain %q", output.String(), detail)
		}
	}

	output.Reset()
	if err := RunWithHome(context.Background(), cfg, manifest, home, []string{root}, opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "manifest: no changes") {
		t.Errorf("idempotent summary = %q", output.String())
	}
}

func TestRunWithHomeDeclineLeavesRemoteStateUnchanged(t *testing.T) {
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
		Confirm: func(string) (bool, error) { return false, nil },
	}
	if err := RunWithHome(context.Background(), cfg, manifest, home, []string{"example/skills"}, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatalf("manifest changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".chai", "sources")); !os.IsNotExist(err) {
		t.Fatalf("persistent cache hierarchy changed: %v", err)
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
	for _, detail := range []string{"https://github.com/example/skills", "Selected skills (1)", "  one", "Manifest", "Add GitHub source", "Platforms", "Cursor"} {
		if !strings.Contains(output.String(), detail) {
			t.Errorf("summary %q does not contain %q", output.String(), detail)
		}
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
	for _, detail := range []string{"Selected skills (2)", "  one\n  two", "Add skills two"} {
		if !strings.Contains(output.String(), detail) {
			t.Errorf("merge summary %q does not contain %q", output.String(), detail)
		}
	}
	loaded, err = config.Load(manifest)
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunWithHome(context.Background(), loaded, manifest, home, mergeArgs, opts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Manifest\n\n  No changes") {
		t.Errorf("idempotent summary = %q", output.String())
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

func TestRunWithHomeRemoteAddReportsProgressAndStructuredSummary(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	cfg := &config.Config{Platforms: []string{"claude", "cursor"}}
	source := testRepository(t)
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	var labels []string
	var summary string
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Discover: func(ctx context.Context, _ githubskill.Identity, repository string) (githubskill.Discovery, error) {
			return githubskill.Discover(ctx, cloneURL, repository)
		},
		Progress: func(label string, operation func() error) error {
			labels = append(labels, label)
			return operation()
		},
		Confirm: func(value string) (bool, error) {
			summary = value
			return false, nil
		},
	}

	if err := RunWithHome(context.Background(), cfg, manifest, home, []string{"example/skills", "--skill", "one"}, opts); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(labels, []string{"Inspecting example/skills", "Fetching 1 selected skill"}) {
		t.Fatalf("progress labels = %v", labels)
	}
	for _, detail := range []string{
		"GitHub source\n",
		"  https://github.com/example/skills",
		"Selected skills (1)\n",
		"  one",
		"Manifest\n",
		"  Add GitHub source",
		"Platforms\n",
		"  Claude · Cursor",
	} {
		if !strings.Contains(summary, detail) {
			t.Errorf("summary missing %q:\n%s", detail, summary)
		}
	}
	if strings.Contains(summary, ";") {
		t.Errorf("summary should not be a semicolon-delimited sentence:\n%s", summary)
	}
}

func TestRunWithHomeRemoteAddRejectsUnmanagedDestinationsBeforeConfirmation(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	cfg := &config.Config{Platforms: []string{"cursor"}}
	writeSkill(t, filepath.Join(home, ".cursor", "skills", "one"), "one")
	source := testRepository(t)
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	confirmed := false
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Discover: func(ctx context.Context, _ githubskill.Identity, repository string) (githubskill.Discovery, error) {
			return githubskill.Discover(ctx, cloneURL, repository)
		},
		Progress: func(_ string, operation func() error) error { return operation() },
		Confirm: func(string) (bool, error) {
			confirmed = true
			return true, nil
		},
	}

	err := RunWithHome(context.Background(), cfg, manifest, home, []string{"example/skills", "--skill", "one"}, opts)
	if err == nil || !strings.Contains(err.Error(), filepath.Join(home, ".cursor", "skills", "one")) {
		t.Fatalf("collision error = %v", err)
	}
	if confirmed {
		t.Fatal("confirmation ran before destination validation")
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

func TestRunWithHomeRejectsConflictBeforeConfirmationAndPromotion(t *testing.T) {
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	local := filepath.Join(home, "local")
	writeSkill(t, local, "one")
	cfg := &config.Config{Platforms: []string{"cursor"}, Skills: config.Skills{Local: []string{local}}}
	source := testRepository(t)
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()
	confirmed := false
	opts := Options{
		CheckGit: func(context.Context) error { return nil },
		Discover: func(ctx context.Context, _ githubskill.Identity, repository string) (githubskill.Discovery, error) {
			return githubskill.Discover(ctx, cloneURL, repository)
		},
		Confirm: func(string) (bool, error) {
			confirmed = true
			return true, nil
		},
	}
	err := RunWithHome(context.Background(), cfg, manifest, home, []string{"example/skills", "--skill", "one"}, opts)
	if err == nil {
		t.Fatal("expected name conflict")
	}
	if confirmed {
		t.Fatal("confirmation ran before global name validation")
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
