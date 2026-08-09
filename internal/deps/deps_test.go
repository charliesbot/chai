package deps

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliesbot/chai/internal/config"
)

func TestSyncOne_CloneAndPull(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	home := t.TempDir()

	bareRepo := testRepository(t)
	dep := config.Dep{URL: bareRepo}

	// First sync: should clone
	result := SyncOne("myrepo", dep, home)
	if result.Err != nil {
		t.Fatalf("clone failed: %v", result.Err)
	}

	clonedPath := filepath.Join(home, ".chai", "deps", "myrepo")
	if _, err := os.Stat(filepath.Join(clonedPath, "README.md")); err != nil {
		t.Error("README.md not found after clone")
	}

	// Second sync: should pull (no error)
	result = SyncOne("myrepo", dep, home)
	if result.Err != nil {
		t.Fatalf("pull failed: %v", result.Err)
	}
}

func TestSyncOne_InvalidURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	home := t.TempDir()
	result := SyncOne("bad", config.Dep{URL: "https://invalid.example.com/nonexistent.git"}, home)
	if result.Err == nil {
		t.Fatal("expected error for invalid URL")
	}
	if !strings.Contains(result.Err.Error(), "cloning") {
		t.Errorf("error = %q, want it to contain 'cloning'", result.Err)
	}
}

func TestSyncOne_WithBuild(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	home := t.TempDir()

	bareRepo := testRepository(t)

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

func testRepository(t *testing.T) string {
	t.Helper()
	bareRepo := filepath.Join(t.TempDir(), "bare.git")
	run(t, "", "git", "init", "--bare", bareRepo)
	tmp := filepath.Join(t.TempDir(), "work")
	run(t, "", "git", "clone", bareRepo, tmp)
	if err := os.WriteFile(filepath.Join(tmp, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	run(t, tmp, "git", "add", ".")
	run(t, tmp, "git", "-c", "user.name=test", "-c", "user.email=test@test.com", "commit", "-m", "init")
	run(t, tmp, "git", "push")
	return bareRepo
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
