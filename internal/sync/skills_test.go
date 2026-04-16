package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charliesbot/chai/internal/hash"
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
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte("---\nname: reviewer\n---\n"), 0644)

	hashDB := hash.DB{}
	err := syncSkillsAndAgents(
		[]string{"~/dotfiles/ai/skills/*"},
		[]string{"~/dotfiles/ai/subagents/*"},
		home, platform.All(), false, hashDB,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Skills go to skills dir
	claudeSkills := filepath.Join(home, ".claude", "skills")
	if _, err := os.Lstat(filepath.Join(claudeSkills, "web-dev")); err != nil {
		t.Error("web-dev symlink missing from skills dir")
	}

	// Agents are copied to agents dir
	claudeAgents := filepath.Join(home, ".claude", "agents")
	info, err := os.Lstat(filepath.Join(claudeAgents, "reviewer.md"))
	if err != nil {
		t.Error("reviewer.md missing from agents dir")
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Error("reviewer.md should be a copy, not a symlink")
	}
}

func TestResolveFilePatterns(t *testing.T) {
	home := t.TempDir()

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	for _, name := range []string{"reviewer.md", "planner.md", "architect.md"} {
		os.WriteFile(filepath.Join(agentsDir, name), []byte("---\nname: test\n---\n"), 0644)
	}
	// Non-md file should be excluded
	os.WriteFile(filepath.Join(agentsDir, "notes.txt"), []byte("ignore"), 0644)

	patterns := []string{"~/dotfiles/ai/subagents/*"}
	results, err := resolveFilePatterns(patterns, home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("got %d results, want 3: %v", len(results), results)
	}
}

func TestResolveFilePatterns_SkipsHidden(t *testing.T) {
	home := t.TempDir()

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte("---\nname: test\n---\n"), 0644)
	os.WriteFile(filepath.Join(agentsDir, ".hidden.md"), []byte("---\nname: hidden\n---\n"), 0644)

	patterns := []string{"~/dotfiles/ai/subagents/*"}
	results, err := resolveFilePatterns(patterns, home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("got %d results, want 1: %v", len(results), results)
	}
}

func TestSyncFileCopies_CreatesFiles(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	content := "---\nname: reviewer\n---\nYou review code."
	reviewerPath := filepath.Join(agentsDir, "reviewer.md")
	os.WriteFile(reviewerPath, []byte(content), 0644)

	destDir := filepath.Join(home, ".claude", "agents")

	err := syncFileCopies([]string{reviewerPath}, destDir, hashDB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dest := filepath.Join(destDir, "reviewer.md")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading copied file: %v", err)
	}
	if string(got) != content {
		t.Errorf("content = %q, want %q", string(got), content)
	}
	// Must be a regular file, not a symlink
	info, _ := os.Lstat(dest)
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("expected regular file, got symlink")
	}
	// Hash should be stored
	if _, ok := hashDB[dest]; !ok {
		t.Error("hash not stored for copied file")
	}
}

func TestSyncFileCopies_RemovesStaleChaiManaged(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(agentsDir, "old.md"), []byte("b"), 0644)

	destDir := filepath.Join(home, ".claude", "agents")

	// First sync with both
	err := syncFileCopies([]string{
		filepath.Join(agentsDir, "reviewer.md"),
		filepath.Join(agentsDir, "old.md"),
	}, destDir, hashDB)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Second sync without old.md
	err = syncFileCopies([]string{
		filepath.Join(agentsDir, "reviewer.md"),
	}, destDir, hashDB)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "old.md")); !os.IsNotExist(err) {
		t.Error("stale chai-managed file old.md should have been removed")
	}
	if _, err := os.Stat(filepath.Join(destDir, "reviewer.md")); err != nil {
		t.Error("reviewer.md should still exist")
	}
}

func TestSyncFileCopies_LeavesUserCreatedFiles(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte("a"), 0644)

	destDir := filepath.Join(home, ".claude", "agents")

	// First sync
	err := syncFileCopies([]string{
		filepath.Join(agentsDir, "reviewer.md"),
	}, destDir, hashDB)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	// User creates their own agent
	os.WriteFile(filepath.Join(destDir, "my-custom-agent.md"), []byte("user agent"), 0644)

	// Second sync — should leave user's file alone
	err = syncFileCopies([]string{
		filepath.Join(agentsDir, "reviewer.md"),
	}, destDir, hashDB)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "my-custom-agent.md"))
	if err != nil {
		t.Fatal("user-created agent was deleted")
	}
	if string(data) != "user agent" {
		t.Error("user-created agent content was modified")
	}
}

func TestSyncFileCopies_WarnsOnUserEditedAgent(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte("v1"), 0644)

	destDir := filepath.Join(home, ".claude", "agents")

	// First sync
	syncFileCopies([]string{filepath.Join(agentsDir, "reviewer.md")}, destDir, hashDB)

	// User edits the copied agent
	os.WriteFile(filepath.Join(destDir, "reviewer.md"), []byte("user edited"), 0644)

	// Update source
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte("v2"), 0644)

	// Second sync — should still overwrite (agents are chai-owned) but hash should have been tracked
	err := syncFileCopies([]string{filepath.Join(agentsDir, "reviewer.md")}, destDir, hashDB)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	// File should be updated to v2
	data, _ := os.ReadFile(filepath.Join(destDir, "reviewer.md"))
	if string(data) != "v2" {
		t.Errorf("content = %q, want %q", string(data), "v2")
	}
}

func TestSyncFileCopies_UpdatesContent(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	src := filepath.Join(agentsDir, "reviewer.md")
	os.WriteFile(src, []byte("v1"), 0644)

	destDir := filepath.Join(home, ".claude", "agents")
	syncFileCopies([]string{src}, destDir, hashDB)

	// Update source
	os.WriteFile(src, []byte("v2"), 0644)
	syncFileCopies([]string{src}, destDir, hashDB)

	got, _ := os.ReadFile(filepath.Join(destDir, "reviewer.md"))
	if string(got) != "v2" {
		t.Errorf("content = %q, want %q", string(got), "v2")
	}
}

func TestSyncSkillsAndAgents_SubagentFiles(t *testing.T) {
	home := t.TempDir()

	// Skills are directories
	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	os.MkdirAll(filepath.Join(skillsDir, "web-dev"), 0755)

	// Subagents are .md files
	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte("---\nname: reviewer\n---\n"), 0644)
	os.WriteFile(filepath.Join(agentsDir, "planner.md"), []byte("---\nname: planner\n---\n"), 0644)

	hashDB := hash.DB{}
	err := syncSkillsAndAgents(
		[]string{"~/dotfiles/ai/skills/*"},
		[]string{"~/dotfiles/ai/subagents/*"},
		home, platform.All(), false, hashDB,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Skills go to skills dir
	claudeSkills := filepath.Join(home, ".claude", "skills")
	if _, err := os.Lstat(filepath.Join(claudeSkills, "web-dev")); err != nil {
		t.Error("web-dev symlink missing from skills dir")
	}

	// Subagent files are copied (not symlinked) to agents dir
	claudeAgents := filepath.Join(home, ".claude", "agents")
	for _, name := range []string{"reviewer.md", "planner.md"} {
		path := filepath.Join(claudeAgents, name)
		info, err := os.Lstat(path)
		if err != nil {
			t.Errorf("%s missing from claude agents dir", name)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s should be a copy, not a symlink", name)
		}
	}

	// Gemini too
	geminiAgents := filepath.Join(home, ".gemini", "agents")
	info, err := os.Lstat(filepath.Join(geminiAgents, "reviewer.md"))
	if err != nil {
		t.Error("reviewer.md missing from gemini agents dir")
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Error("reviewer.md should be a copy, not a symlink")
	}
}

func TestSyncSkillsAndAgents_EmptyAgents(t *testing.T) {
	home := t.TempDir()

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	os.MkdirAll(filepath.Join(skillsDir, "web-dev"), 0755)

	// Empty agents dir
	os.MkdirAll(filepath.Join(home, "dotfiles", "ai", "subagents"), 0755)

	hashDB := hash.DB{}
	err := syncSkillsAndAgents(
		[]string{"~/dotfiles/ai/skills/*"},
		[]string{"~/dotfiles/ai/subagents/*"},
		home, platform.All(), false, hashDB,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claudeSkills := filepath.Join(home, ".claude", "skills")
	if _, err := os.Lstat(filepath.Join(claudeSkills, "web-dev")); err != nil {
		t.Error("web-dev symlink missing — empty agents should not affect skills")
	}
}
