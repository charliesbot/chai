package add

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charliesbot/chai/internal/clean"
	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	"github.com/charliesbot/chai/internal/hash"
	chaisync "github.com/charliesbot/chai/internal/sync"
	"github.com/charliesbot/chai/internal/update"
)

func stitchRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "uploadpack.allowFilter", "true")
	writeSkill(t, filepath.Join(repo, "plugins", "stitch-build", "skills", "react-components"), "stitch::react-components")
	writeSkill(t, filepath.Join(repo, "plugins", "stitch-utilities", "skills", "plain"), "plain")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "Stitch fixture")
	redirectGitHubClone(t, repo)
	return repo
}

func TestNamespacedSkillLifecycle(t *testing.T) {
	repo := stitchRepository(t)
	ctx := context.Background()
	home := t.TempDir()
	manifest := filepath.Join(home, "chai.toml")
	instructions := filepath.Join(home, "AGENTS.md")
	if err := os.WriteFile(instructions, []byte("Test instructions\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Platforms: []string{"cursor", "codex"}, Instructions: []string{instructions}}
	var output bytes.Buffer
	if err := RunWithHome(ctx, cfg, manifest, home, []string{"example/skills", "--list"}, Options{Output: &output}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "stitch::react-components") || strings.Contains(output.String(), "invalid") {
		t.Fatalf("unexpected listing: %s", output.String())
	}
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatalf("listing wrote manifest: %v", err)
	}
	id, err := githubskill.ParseInput("example/skills")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(githubskill.CacheDir(home, id)); !os.IsNotExist(err) {
		t.Fatalf("listing promoted cache: %v", err)
	}
	for _, args := range [][]string{
		{"example/skills", "--skill", "stitch::react-components"},
		{"example/skills"},
	} {
		if err := RunWithHome(ctx, cfg, manifest, home, args, Options{}); err != nil {
			t.Fatal(err)
		}
		cfg, err = config.Load(manifest)
		if err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"plain", "stitch::react-components"}; !reflect.DeepEqual(cfg.Skills.GitHub[0].Include, want) {
		t.Fatalf("include = %v, want %v", cfg.Skills.GitHub[0].Include, want)
	}
	cached, err := githubskill.ResolveCached(home, id, []string{"stitch::react-components"})
	if err != nil || len(cached) != 1 || cached[0].Name != "stitch::react-components" {
		t.Fatalf("cached = %+v, err = %v", cached, err)
	}
	sourceFile := filepath.Join(repo, "plugins", "stitch-build", "skills", "react-components", "SKILL.md")
	original, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	destinations := []string{
		filepath.Join(home, ".cursor", "skills", "stitch-react-components", "SKILL.md"),
		filepath.Join(home, ".agents", "skills", "stitch-react-components", "SKILL.md"),
	}
	assertContents := func(want []byte) {
		t.Helper()
		for _, dest := range destinations {
			got, err := os.ReadFile(dest)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("%s: content = %q, err = %v", dest, got, err)
			}
		}
	}
	assertContents(original)
	if err := chaisync.RunWithHome(ctx, cfg, home, chaisync.Options{}); err != nil {
		t.Fatal(err)
	}
	assertContents(original)

	updated := append(append([]byte(nil), original...), []byte("Updated upstream body\n")...)
	if err := os.WriteFile(sourceFile, updated, 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "Update fixture")
	if err := update.RunWithHome(ctx, cfg, home, update.Options{}); err != nil {
		t.Fatal(err)
	}
	assertContents(updated)

	if err := os.WriteFile(destinations[0], []byte("local edits"), 0644); err != nil {
		t.Fatal(err)
	}
	var dirty *chaisync.DirtyError
	if err := chaisync.RunWithHome(ctx, cfg, home, chaisync.Options{}); !errors.As(err, &dirty) {
		t.Fatalf("want dirty-file error, got %v", err)
	}
	got, err := os.ReadFile(destinations[0])
	if err != nil || string(got) != "local edits" {
		t.Fatalf("dirty output overwritten: %q, %v", got, err)
	}

	// Restore through the normal prompt path, then verify stale-skill removal.
	if err := chaisync.RunWithHome(ctx, cfg, home, chaisync.Options{Prompt: func(string) (bool, error) { return true, nil }}); err != nil {
		t.Fatal(err)
	}
	assertContents(updated)
	cfg.Skills.GitHub[0].Include = []string{"plain"}
	if err := chaisync.RunWithHome(ctx, cfg, home, chaisync.Options{}); err != nil {
		t.Fatal(err)
	}
	for _, dest := range destinations {
		if _, err := os.Stat(filepath.Dir(dest)); !os.IsNotExist(err) {
			t.Fatalf("stale output remains: %v", err)
		}
	}
	cfg.Skills.GitHub[0].Include = []string{"plain", "stitch::react-components"}
	if err := chaisync.RunWithHome(ctx, cfg, home, chaisync.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := clean.RunWithHome(ctx, cfg, home, clean.Options{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	assertContents(updated)
	if err := clean.RunWithHome(ctx, cfg, home, clean.Options{}); err != nil {
		t.Fatal(err)
	}
	for _, dest := range destinations {
		if _, err := os.Stat(filepath.Dir(dest)); !os.IsNotExist(err) {
			t.Fatalf("clean left output: %v", err)
		}
	}
	db, err := hash.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, dest := range destinations {
		if _, exists := db[filepath.Dir(dest)]; exists {
			t.Fatalf("clean left hash for %s", dest)
		}
	}
}

func TestNamespacedAddRejectsDestinationCollisionBeforeMutation(t *testing.T) {
	stitchRepository(t)
	home := t.TempDir()
	local := filepath.Join(home, "local")
	writeSkill(t, local, "stitch-react-components")
	cfg := &config.Config{Platforms: []string{"cursor"}, Skills: config.Skills{Local: []string{local}}}
	manifest := filepath.Join(home, "chai.toml")
	err := RunWithHome(context.Background(), cfg, manifest, home, []string{"example/skills"}, Options{})
	if err == nil || !strings.Contains(err.Error(), "destination") {
		t.Fatalf("want destination collision, got %v", err)
	}
	id, _ := githubskill.ParseInput("example/skills")
	for _, path := range []string{manifest, githubskill.CacheDir(home, id), filepath.Join(home, ".cursor")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("mutated %s: %v", path, err)
		}
	}
}

func TestNamespacedLocalAdd(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "local-skill")
	writeSkill(t, root, "stitch::react-components")
	cfg := &config.Config{Platforms: []string{"cursor"}}
	if err := RunWithHome(context.Background(), cfg, filepath.Join(home, "chai.toml"), home, []string{root}, Options{}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(home, ".cursor", "skills", "stitch-react-components", "SKILL.md")
	got, err := os.ReadFile(dest)
	if err != nil || !strings.Contains(string(got), "name: stitch::react-components") {
		t.Fatalf("local skill content = %q, err = %v", got, err)
	}
}

func TestNamespacedAddRejectsUnmanagedDestination(t *testing.T) {
	stitchRepository(t)
	home := t.TempDir()
	dest := filepath.Join(home, ".cursor", "skills", "stitch-react-components")
	writeSkill(t, dest, "user-owned")
	cfg := &config.Config{Platforms: []string{"cursor"}}
	manifest := filepath.Join(home, "chai.toml")
	err := RunWithHome(context.Background(), cfg, manifest, home, []string{"example/skills"}, Options{})
	if err == nil || !strings.Contains(err.Error(), "not managed by chai") || !strings.Contains(err.Error(), dest) {
		t.Fatalf("want unmanaged destination error, got %v", err)
	}
	id, _ := githubskill.ParseInput("example/skills")
	for _, path := range []string{manifest, githubskill.CacheDir(home, id)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("mutated %s: %v", path, err)
		}
	}
	got, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil || !strings.Contains(string(got), "name: user-owned") {
		t.Fatalf("unmanaged content = %q, err = %v", got, err)
	}
}

func TestNamespacedCollisionPreflightAcrossSources(t *testing.T) {
	for _, kind := range []string{"local-local", "local-remote", "remote-remote", "same-repository"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			cfg := &config.Config{Platforms: []string{"cursor"}}
			for i, name := range []string{"stitch::react-components", "stitch-react-components"} {
				if kind == "local-local" || kind == "local-remote" && i == 0 {
					root := filepath.Join(home, name)
					writeSkill(t, root, name)
					cfg.Skills.Local = append(cfg.Skills.Local, root)
				} else if kind == "same-repository" && i == 1 {
					cfg.Skills.GitHub[0].Include = append(cfg.Skills.GitHub[0].Include, name)
				} else {
					cfg.Skills.GitHub = append(cfg.Skills.GitHub, config.GitHubSkills{
						URL:     []string{"https://github.com/example/one", "https://github.com/example/two"}[i],
						Include: []string{name},
					})
				}
			}
			ctx := context.Background()
			err := chaisync.RunWithHome(ctx, cfg, home, chaisync.Options{})
			if err == nil || !strings.Contains(err.Error(), "destination") {
				t.Fatalf("sync: want destination conflict before resolving caches, got %v", err)
			}
			err = update.RunWithHome(ctx, cfg, home, update.Options{
				CheckGit: func(context.Context) error { t.Fatal("Git checked before collision validation"); return nil },
			})
			if err == nil || !strings.Contains(err.Error(), "destination") {
				t.Fatalf("update: want destination conflict, got %v", err)
			}
			for _, dir := range []string{".cursor", ".chai"} {
				if _, err := os.Stat(filepath.Join(home, dir)); !os.IsNotExist(err) {
					t.Fatalf("preflight wrote %s: %v", dir, err)
				}
			}
		})
	}
}
