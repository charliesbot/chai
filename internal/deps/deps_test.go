package deps

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliesbot/chai/internal/config"
)

func TestSyncWithHome_CloneAndPull(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	home := t.TempDir()

	bareRepo := filepath.Join(t.TempDir(), "bare.git")
	run(t, "", "git", "init", "--bare", bareRepo)

	tmp := filepath.Join(t.TempDir(), "work")
	run(t, "", "git", "clone", bareRepo, tmp)
	os.WriteFile(filepath.Join(tmp, "README.md"), []byte("hello"), 0644)
	run(t, tmp, "git", "add", ".")
	run(t, tmp, "git", "-c", "user.name=test", "-c", "user.email=test@test.com", "commit", "-m", "init")
	run(t, tmp, "git", "push")

	depMap := map[string]config.Dep{
		"myrepo": {URL: bareRepo},
	}

	// First sync: should clone
	err := SyncWithHome(depMap, home)
	if err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	clonedPath := filepath.Join(home, ".chai", "deps", "myrepo")
	if _, err := os.Stat(filepath.Join(clonedPath, "README.md")); err != nil {
		t.Error("README.md not found after clone")
	}

	// Second sync: should pull (no error)
	err = SyncWithHome(depMap, home)
	if err != nil {
		t.Fatalf("pull failed: %v", err)
	}
}

func TestSyncWithHome_NoDeps(t *testing.T) {
	home := t.TempDir()
	err := SyncWithHome(nil, home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncWithHome_InvalidURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	home := t.TempDir()
	depMap := map[string]config.Dep{
		"bad": {URL: "https://invalid.example.com/nonexistent.git"},
	}

	err := SyncWithHome(depMap, home)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	if !strings.Contains(err.Error(), "cloning") {
		t.Errorf("error = %q, want it to contain 'cloning'", err.Error())
	}
}

func TestSyncOne_WithBuild(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	home := t.TempDir()

	bareRepo := filepath.Join(t.TempDir(), "bare.git")
	run(t, "", "git", "init", "--bare", bareRepo)

	tmp := filepath.Join(t.TempDir(), "work")
	run(t, "", "git", "clone", bareRepo, tmp)
	os.WriteFile(filepath.Join(tmp, "README.md"), []byte("hello"), 0644)
	run(t, tmp, "git", "add", ".")
	run(t, tmp, "git", "-c", "user.name=test", "-c", "user.email=test@test.com", "commit", "-m", "init")
	run(t, tmp, "git", "push")

	dep := config.Dep{URL: bareRepo, Build: "touch built.txt"}

	// First clone: should run build
	result := SyncOne("myrepo", dep, home)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Action != ActionCloned {
		t.Errorf("action = %q, want %q", result.Action, ActionCloned)
	}
	if !result.Built {
		t.Error("expected Built = true on first clone")
	}

	builtFile := filepath.Join(home, ".chai", "deps", "myrepo", "built.txt")
	if _, err := os.Stat(builtFile); err != nil {
		t.Error("built.txt not found — build command didn't run")
	}

	// Second sync (pull): should NOT run build
	result = SyncOne("myrepo", dep, home)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Built {
		t.Error("build should not run on pull")
	}
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running %s %v: %v\n%s", name, args, err, out)
	}
}
