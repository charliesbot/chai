package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncSymlinks_CreatesLinks(t *testing.T) {
	home := t.TempDir()

	// Create source skill directories
	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	for _, name := range []string{"web-dev", "android-dev"} {
		os.MkdirAll(filepath.Join(skillsDir, name), 0755)
	}

	sources := []string{
		filepath.Join(skillsDir, "web-dev"),
		filepath.Join(skillsDir, "android-dev"),
	}

	destDir := filepath.Join(home, ".claude", "skills")

	err := syncSymlinks(sources, destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify symlinks exist and point to correct targets
	for _, name := range []string{"web-dev", "android-dev"} {
		link := filepath.Join(destDir, name)
		target, err := os.Readlink(link)
		if err != nil {
			t.Errorf("reading symlink %s: %v", link, err)
			continue
		}
		expected := filepath.Join(skillsDir, name)
		if target != expected {
			t.Errorf("symlink %s → %q, want %q", name, target, expected)
		}
	}
}

func TestSyncSymlinks_RemovesStale(t *testing.T) {
	home := t.TempDir()

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	os.MkdirAll(filepath.Join(skillsDir, "web-dev"), 0755)
	os.MkdirAll(filepath.Join(skillsDir, "old-skill"), 0755)

	destDir := filepath.Join(home, ".claude", "skills")

	// First sync with two skills
	err := syncSymlinks([]string{
		filepath.Join(skillsDir, "web-dev"),
		filepath.Join(skillsDir, "old-skill"),
	}, destDir)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Second sync with only one skill — old-skill should be removed
	err = syncSymlinks([]string{
		filepath.Join(skillsDir, "web-dev"),
	}, destDir)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(destDir, "old-skill")); !os.IsNotExist(err) {
		t.Error("stale symlink old-skill should have been removed")
	}
	if _, err := os.Lstat(filepath.Join(destDir, "web-dev")); err != nil {
		t.Error("web-dev symlink should still exist")
	}
}

func TestSyncSymlinks_ReplacesbrokenLinks(t *testing.T) {
	home := t.TempDir()

	destDir := filepath.Join(home, ".claude", "skills")
	os.MkdirAll(destDir, 0755)

	// Create a broken symlink
	os.Symlink("/nonexistent/old-path", filepath.Join(destDir, "web-dev"))

	// Create the real source
	src := filepath.Join(home, "skills", "web-dev")
	os.MkdirAll(src, 0755)

	err := syncSymlinks([]string{src}, destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	target, err := os.Readlink(filepath.Join(destDir, "web-dev"))
	if err != nil {
		t.Fatalf("reading symlink: %v", err)
	}
	if target != src {
		t.Errorf("symlink → %q, want %q", target, src)
	}
}

func TestSyncSymlinks_RefusesToOverwriteNonSymlink(t *testing.T) {
	home := t.TempDir()

	destDir := filepath.Join(home, ".claude", "skills")
	// Create a real directory (not a symlink) at the target
	os.MkdirAll(filepath.Join(destDir, "web-dev"), 0755)

	src := filepath.Join(home, "skills", "web-dev")
	os.MkdirAll(src, 0755)

	err := syncSymlinks([]string{src}, destDir)
	if err == nil {
		t.Fatal("expected error when target is a real directory")
	}
}

func TestResolvePatterns(t *testing.T) {
	home := t.TempDir()

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	for _, name := range []string{"web-dev", "android-dev", "slidev"} {
		os.MkdirAll(filepath.Join(skillsDir, name), 0755)
	}
	// Create a file (not a dir) — should be filtered out
	os.WriteFile(filepath.Join(skillsDir, "README.md"), []byte("hi"), 0644)

	patterns := []string{"~/dotfiles/ai/skills/*"}
	results, err := resolvePatterns(patterns, home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("got %d results, want 3: %v", len(results), results)
	}
}
