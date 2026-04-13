package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charliesbot/chai/internal/platform"
)

func TestSyncSymlinks_CreatesLinks(t *testing.T) {
	home := t.TempDir()

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

	err := syncSymlinks([]string{
		filepath.Join(skillsDir, "web-dev"),
		filepath.Join(skillsDir, "old-skill"),
	}, destDir)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}

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

func TestSyncSymlinks_ReplacesBrokenLinks(t *testing.T) {
	home := t.TempDir()

	destDir := filepath.Join(home, ".claude", "skills")
	os.MkdirAll(destDir, 0755)
	os.Symlink("/nonexistent/old-path", filepath.Join(destDir, "web-dev"))

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

func TestResolvePatterns_DirectoryPath(t *testing.T) {
	home := t.TempDir()

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	os.MkdirAll(skillsDir, 0755)
	os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte("skill"), 0644)

	// A bare directory path should resolve to the directory itself
	patterns := []string{"~/dotfiles/ai/skills"}
	results, err := resolvePatterns(patterns, home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("got %d results, want 1: %v", len(results), results)
	}
	if len(results) > 0 && results[0] != skillsDir {
		t.Errorf("got %q, want %q", results[0], skillsDir)
	}
}

func TestResolvePatterns_GlobChildren(t *testing.T) {
	home := t.TempDir()

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	for _, name := range []string{"web-dev", "android-dev"} {
		os.MkdirAll(filepath.Join(skillsDir, name), 0755)
	}

	// Use explicit glob to get children
	patterns := []string{"~/dotfiles/ai/skills/*"}
	results, err := resolvePatterns(patterns, home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("got %d results, want 2: %v", len(results), results)
	}
}

func TestSyncSkillsAndAgents_SeparateDirectories(t *testing.T) {
	home := t.TempDir()

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	os.MkdirAll(filepath.Join(skillsDir, "web-dev"), 0755)

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(filepath.Join(agentsDir, "reviewer"), 0755)

	err := syncSkillsAndAgents(
		[]string{"~/dotfiles/ai/skills/*"},
		[]string{"~/dotfiles/ai/subagents/*"},
		home, platform.All(), false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Skills go to skills dir
	claudeSkills := filepath.Join(home, ".claude", "skills")
	if _, err := os.Lstat(filepath.Join(claudeSkills, "web-dev")); err != nil {
		t.Error("web-dev symlink missing from skills dir")
	}

	// Agents go to subagents dir
	claudeAgents := filepath.Join(home, ".claude", "subagents")
	if _, err := os.Lstat(filepath.Join(claudeAgents, "reviewer")); err != nil {
		t.Error("reviewer symlink missing from subagents dir")
	}
}

func TestSyncSkillsAndAgents_EmptyAgents(t *testing.T) {
	home := t.TempDir()

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	os.MkdirAll(filepath.Join(skillsDir, "web-dev"), 0755)

	// Empty agents dir
	os.MkdirAll(filepath.Join(home, "dotfiles", "ai", "subagents"), 0755)

	err := syncSkillsAndAgents(
		[]string{"~/dotfiles/ai/skills/*"},
		[]string{"~/dotfiles/ai/subagents/*"},
		home, platform.All(), false,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claudeSkills := filepath.Join(home, ".claude", "skills")
	if _, err := os.Lstat(filepath.Join(claudeSkills, "web-dev")); err != nil {
		t.Error("web-dev symlink missing — empty agents should not affect skills")
	}
}
