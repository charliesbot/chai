package sync

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
)

func TestSyncSkills_ReportsChangesAndCollapsesUnmanagedSkills(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}
	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	for _, skill := range []struct{ name, content string }{
		{"agents-md", "v1"}, {"old-skill", "v1"}, {"web-dev", "v1"},
	} {
		writeSkillDir(t, filepath.Join(skillsDir, skill.name), skill.content)
	}
	syncCursorSkills(t, home, hashDB)

	if err := os.RemoveAll(filepath.Join(skillsDir, "old-skill")); err != nil {
		t.Fatalf("removing old skill: %v", err)
	}
	writeSkillDir(t, filepath.Join(skillsDir, "adaptive"), "v1")
	writeSkillDir(t, filepath.Join(skillsDir, "web-dev"), "v2")

	destDir := filepath.Join(home, ".cursor", "skills")
	for _, name := range []string{"custom-one", "custom-two"} {
		writeSkillDir(t, filepath.Join(destDir, name), "user managed")
	}

	output := syncCursorSkills(t, home, hashDB)

	assertOutputContains(t, output,
		"skills",
		"1 added",
		"1 updated",
		"1 removed",
		"1 unchanged",
		"+ adaptive",
		"~ web-dev",
		"- old-skill",
		"2 unmanaged skills preserved",
	)
	if strings.Contains(output, "custom-") || strings.Contains(output, "agents-md") {
		t.Errorf("output should omit preserved and unchanged skill names:\n%s", output)
	}
}

func TestSyncSkills_ReportsCompletedChangesWhenLaterCopyFails(t *testing.T) {
	home := t.TempDir()
	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	brokenSkill := filepath.Join(skillsDir, "broken-skill")
	writeSkillDir(t, brokenSkill, "unreadable")
	skillFile := filepath.Join(brokenSkill, "SKILL.md")
	if err := os.Chmod(skillFile, 0000); err != nil {
		t.Fatalf("making skill unreadable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(skillFile, 0644) })

	destDir := filepath.Join(home, ".cursor", "skills")
	staleSkill := filepath.Join(destDir, "stale-skill")
	writeSkillDir(t, staleSkill, "stale")
	hashDB := hash.DB{staleSkill: "managed-hash"}

	output := syncCursorSkills(t, home, hashDB)

	assertOutputContains(t, output, "1 removed", "- stale-skill")
}

func TestSyncSkills_CapsChangedNames(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}
	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		writeSkillDir(t, filepath.Join(skillsDir, name), name)
	}

	output := syncCursorSkills(t, home, hashDB)

	if !strings.Contains(output, "7 added") || !strings.Contains(output, "... 2 more") ||
		strings.Contains(output, "+ f") || strings.Contains(output, "+ g") {
		t.Fatalf("output should show at most five changed names:\n%s", output)
	}
}

func assertOutputContains(t *testing.T, output string, expected ...string) {
	t.Helper()
	for _, value := range expected {
		if !strings.Contains(output, value) {
			t.Errorf("output missing %q:\n%s", value, output)
		}
	}
}

func syncCursorSkills(t *testing.T, home string, hashDB hash.DB) string {
	t.Helper()
	output, err := captureStdout(t, func() error {
		return syncSkills(
			[]string{filepath.Join(home, "dotfiles", "ai", "skills", "*")},
			home, platform.ForNames([]string{"cursor"}), false, hashDB,
		)
	})
	if err != nil {
		t.Fatalf("syncing skills: %v", err)
	}
	return output
}

func captureStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()

	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating stdout pipe: %v", err)
	}
	os.Stdout = writer
	runErr := run()
	os.Stdout = previous
	_ = writer.Close()
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading stdout: %v", err)
	}
	_ = reader.Close()
	return string(output), runErr
}

func writeSkillDir(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("creating skill directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("writing SKILL.md: %v", err)
	}
}

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

func TestSyncSkills_CopiesCursorSkills(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	skill := filepath.Join(home, "dotfiles", "ai", "skills", "web-dev")
	os.MkdirAll(skill, 0755)
	os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("skill web-dev"), 0644)

	if err := syncSkills([]string{"~/dotfiles/ai/skills/*"}, home, platform.ForNames([]string{"cursor"}), false, hashDB); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dest := filepath.Join(home, ".cursor", "skills", "web-dev")
	data, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading copied cursor skill: %v", err)
	}
	if string(data) != "skill web-dev" {
		t.Fatalf("content = %q, want %q", string(data), "skill web-dev")
	}
	if _, ok := hashDB[dest]; !ok {
		t.Fatal("cursor skill hash was not recorded")
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

func TestSyncSkills_CopiesRootSkillMDAsNamedSkill(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}

	repo := filepath.Join(home, ".chai", "deps", "herdr")
	os.MkdirAll(filepath.Join(repo, "vendor"), 0755)
	os.WriteFile(filepath.Join(repo, "SKILL.md"), []byte("herdr skill"), 0644)
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("repo readme"), 0644)
	os.WriteFile(filepath.Join(repo, "vendor", "large.txt"), []byte("do not copy"), 0644)

	if err := syncSkills([]string{"@herdr/SKILL.md"}, home, platform.ForNames([]string{"codex"}), false, hashDB); err != nil {
		t.Fatalf("sync: %v", err)
	}

	dest := filepath.Join(home, ".agents", "skills", "herdr")
	data, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading copied SKILL.md: %v", err)
	}
	if string(data) != "herdr skill" {
		t.Errorf("content = %q, want %q", string(data), "herdr skill")
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); !os.IsNotExist(err) {
		t.Error("repo sibling files should not be copied for file-backed skills")
	}
	if _, err := os.Stat(filepath.Join(dest, "vendor")); !os.IsNotExist(err) {
		t.Error("repo sibling directories should not be copied for file-backed skills")
	}
	if _, ok := hashDB[dest]; !ok {
		t.Fatal("skill hash was not recorded")
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

func TestSyncSkills_CopiesSkillDirectories(t *testing.T) {
	home := t.TempDir()

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	webDevSrc := filepath.Join(skillsDir, "web-dev")
	os.MkdirAll(webDevSrc, 0755)
	os.WriteFile(filepath.Join(webDevSrc, "SKILL.md"), []byte("web"), 0644)

	hashDB := hash.DB{}
	err := syncSkills(
		[]string{"~/dotfiles/ai/skills/*"},
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

func TestSyncSkills_EmptyAgentsAreIrrelevant(t *testing.T) {
	home := t.TempDir()

	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	webDevSrc := filepath.Join(skillsDir, "web-dev")
	os.MkdirAll(webDevSrc, 0755)
	os.WriteFile(filepath.Join(webDevSrc, "SKILL.md"), []byte("web"), 0644)

	// Empty agents dir
	os.MkdirAll(filepath.Join(home, "dotfiles", "ai", "subagents"), 0755)

	hashDB := hash.DB{}
	err := syncSkills(
		[]string{"~/dotfiles/ai/skills/*"},
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
