package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
)

func TestSyncDirCopies_CreatesCopies(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	for _, name := range []string{"web-dev", "android-dev"} {
		dir := filepath.Join(skillsDir, name)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("skill "+name), 0644)
	}

	sources := []string{
		filepath.Join(skillsDir, "web-dev"),
		filepath.Join(skillsDir, "android-dev"),
	}

	destDir := filepath.Join(home, ".claude", "skills")

	err := syncDirCopies(sources, destDir, hashDB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, name := range []string{"web-dev", "android-dev"} {
		dest := filepath.Join(destDir, name)
		info, err := os.Lstat(dest)
		if err != nil {
			t.Errorf("missing %s: %v", name, err)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("%s should be a copy, not a symlink", name)
		}
		data, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
		if err != nil {
			t.Errorf("reading copied SKILL.md: %v", err)
		}
		if string(data) != "skill "+name {
			t.Errorf("content = %q, want %q", string(data), "skill "+name)
		}
		if _, ok := hashDB[dest]; !ok {
			t.Errorf("hash not stored for %s", name)
		}
	}
}

func TestSyncDirCopies_CopiesNestedFiles(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	src := filepath.Join(home, "dotfiles", "ai", "skills", "web-dev")
	os.MkdirAll(filepath.Join(src, "resources", "templates"), 0755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("top"), 0644)
	os.WriteFile(filepath.Join(src, "resources", "helper.md"), []byte("helper"), 0644)
	os.WriteFile(filepath.Join(src, "resources", "templates", "page.html"), []byte("<html/>"), 0644)

	destDir := filepath.Join(home, ".claude", "skills")
	if err := syncDirCopies([]string{src}, destDir, hashDB); err != nil {
		t.Fatalf("sync: %v", err)
	}

	dest := filepath.Join(destDir, "web-dev")
	cases := map[string]string{
		"SKILL.md":                      "top",
		"resources/helper.md":           "helper",
		"resources/templates/page.html": "<html/>",
	}
	for rel, want := range cases {
		data, err := os.ReadFile(filepath.Join(dest, rel))
		if err != nil {
			t.Errorf("reading %s: %v", rel, err)
			continue
		}
		if string(data) != want {
			t.Errorf("%s = %q, want %q", rel, string(data), want)
		}
	}
}

func TestSyncDirCopies_RemovesStaleChaiManaged(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	for _, name := range []string{"web-dev", "old-skill"} {
		dir := filepath.Join(skillsDir, name)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0644)
	}

	destDir := filepath.Join(home, ".claude", "skills")

	if err := syncDirCopies([]string{
		filepath.Join(skillsDir, "web-dev"),
		filepath.Join(skillsDir, "old-skill"),
	}, destDir, hashDB); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	if err := syncDirCopies([]string{
		filepath.Join(skillsDir, "web-dev"),
	}, destDir, hashDB); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "old-skill")); !os.IsNotExist(err) {
		t.Error("stale chai-managed skill old-skill should have been removed")
	}
	if _, err := os.Stat(filepath.Join(destDir, "web-dev")); err != nil {
		t.Error("web-dev should still exist")
	}
	if _, ok := hashDB[filepath.Join(destDir, "old-skill")]; ok {
		t.Error("old-skill hash should have been deleted from hashDB")
	}
}

func TestSyncDirCopies_LeavesUserCreatedSkills(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	src := filepath.Join(home, "dotfiles", "ai", "skills", "web-dev")
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("x"), 0644)

	destDir := filepath.Join(home, ".claude", "skills")

	if err := syncDirCopies([]string{src}, destDir, hashDB); err != nil {
		t.Fatalf("sync: %v", err)
	}

	userSkill := filepath.Join(destDir, "my-skill")
	os.MkdirAll(userSkill, 0755)
	os.WriteFile(filepath.Join(userSkill, "SKILL.md"), []byte("user"), 0644)

	if err := syncDirCopies([]string{src}, destDir, hashDB); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(userSkill, "SKILL.md"))
	if err != nil {
		t.Fatal("user-created skill was deleted")
	}
	if string(data) != "user" {
		t.Error("user-created skill content was modified")
	}
}

func TestSyncDirCopies_UpdatesContentOnResync(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	src := filepath.Join(home, "dotfiles", "ai", "skills", "web-dev")
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("v1"), 0644)

	destDir := filepath.Join(home, ".claude", "skills")
	if err := syncDirCopies([]string{src}, destDir, hashDB); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	firstHash := hashDB[filepath.Join(destDir, "web-dev")]

	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("v2"), 0644)
	if err := syncDirCopies([]string{src}, destDir, hashDB); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(destDir, "web-dev", "SKILL.md"))
	if string(data) != "v2" {
		t.Errorf("content = %q, want %q", string(data), "v2")
	}
	if hashDB[filepath.Join(destDir, "web-dev")] == firstHash {
		t.Error("hash should have changed after source content changed")
	}
}

func TestSyncDirCopies_RemovesFilesDeletedFromSource(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	src := filepath.Join(home, "dotfiles", "ai", "skills", "web-dev")
	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(src, "extra.md"), []byte("extra"), 0644)

	destDir := filepath.Join(home, ".claude", "skills")
	if err := syncDirCopies([]string{src}, destDir, hashDB); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	os.Remove(filepath.Join(src, "extra.md"))
	if err := syncDirCopies([]string{src}, destDir, hashDB); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "web-dev", "extra.md")); !os.IsNotExist(err) {
		t.Error("extra.md should have been removed when deleted from source")
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
	webDevSrc := filepath.Join(skillsDir, "web-dev")
	os.MkdirAll(webDevSrc, 0755)
	os.WriteFile(filepath.Join(webDevSrc, "SKILL.md"), []byte("web"), 0644)

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

	// Skills are copied to skills dir
	claudeSkills := filepath.Join(home, ".claude", "skills")
	webDevDest := filepath.Join(claudeSkills, "web-dev")
	info, err := os.Lstat(webDevDest)
	if err != nil {
		t.Error("web-dev missing from skills dir")
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Error("web-dev should be a copy, not a symlink")
	}

	// Agents are copied to agents dir
	claudeAgents := filepath.Join(home, ".claude", "agents")
	info, err = os.Lstat(filepath.Join(claudeAgents, "reviewer.md"))
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
	webDevSrc := filepath.Join(skillsDir, "web-dev")
	os.MkdirAll(webDevSrc, 0755)
	os.WriteFile(filepath.Join(webDevSrc, "SKILL.md"), []byte("web"), 0644)

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

	// Skills are copied to skills dir (not symlinked)
	claudeSkills := filepath.Join(home, ".claude", "skills")
	webDevDest := filepath.Join(claudeSkills, "web-dev")
	info, err := os.Lstat(webDevDest)
	if err != nil {
		t.Error("web-dev missing from skills dir")
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Error("web-dev should be a copy, not a symlink")
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
	info, err = os.Lstat(filepath.Join(geminiAgents, "reviewer.md"))
	if err != nil {
		t.Error("reviewer.md missing from gemini agents dir")
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Error("reviewer.md should be a copy, not a symlink")
	}
}

func TestSyncSkillsAndAgents_EmptyAgents(t *testing.T) {
	home := t.TempDir()

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	webDevSrc := filepath.Join(skillsDir, "web-dev")
	os.MkdirAll(webDevSrc, 0755)
	os.WriteFile(filepath.Join(webDevSrc, "SKILL.md"), []byte("web"), 0644)

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
		t.Error("web-dev missing — empty agents should not affect skills")
	}
}
