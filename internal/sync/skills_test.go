package sync

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/skill"
)

func TestItemChangesSummary_ColorsChangeTypes(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	changes := newItemChanges()
	changes.record("added", itemAdded)
	changes.record("updated", itemUpdated)
	changes.record("removed", itemRemoved)
	changes.record("unchanged", itemUnchanged)

	assertOutputContains(t, changes.summary(),
		"\x1b[38;5;42m1 added\x1b[0m",
		"\x1b[38;5;220m1 updated\x1b[0m",
		"\x1b[38;5;196m1 removed\x1b[0m",
		"\x1b[38;5;241m1 unchanged\x1b[0m",
	)
}

func TestSyncSkills_ReportsChangesAndCollapsesUnmanagedSkills(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

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
		"\x1b[38;5;42m+ adaptive\x1b[0m",
		"\x1b[38;5;220m~ web-dev\x1b[0m",
		"\x1b[38;5;196m- old-skill\x1b[0m",
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
	staleHash, err := dirHash(staleSkill)
	if err != nil {
		t.Fatal(err)
	}
	hashDB := hash.DB{staleSkill: staleHash}

	output, err := captureStdout(t, func() error {
		return syncResolvedSkills(
			[]skill.Source{{Name: "broken-skill", Path: brokenSkill}}, home,
			[]platform.Platform{{Name: "Cursor", SkillsDir: ".cursor/skills"}},
			Options{Force: true}, hashDB,
		)
	})
	if err == nil {
		t.Fatal("expected copy failure")
	}

	assertOutputContains(t, output, "1 removed", "- stale-skill")
}

func TestSyncSkills_ReportsEveryChangedName(t *testing.T) {
	home := t.TempDir()
	hashDB := hash.DB{}
	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		writeSkillDir(t, filepath.Join(skillsDir, name), name)
	}

	output := syncCursorSkills(t, home, hashDB)

	for _, want := range []string{"7 added", "+ a", "+ b", "+ c", "+ d", "+ e", "+ f", "+ g"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "more") {
		t.Errorf("output should not truncate changed names:\n%s", output)
	}
}

func TestSyncSkillCopiesRejectsDirtyManagedDestination(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "source")
	destDir := filepath.Join(home, "dest")
	writeSkillDir(t, source, "source")
	writeSkillDir(t, filepath.Join(destDir, "chosen"), "edited")
	dest := filepath.Join(destDir, "chosen")
	hashDB := hash.DB{dest: "previous-hash"}

	_, err := syncSkillCopies([]skill.Source{{Path: source, Name: "chosen"}}, destDir, hashDB, Options{})
	var dirty *DirtyError
	if !errors.As(err, &dirty) {
		t.Fatalf("dirty error = %v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if readErr != nil || !strings.Contains(string(data), "edited") {
		t.Fatalf("dirty destination changed: data=%q err=%v", data, readErr)
	}
}

func TestSyncSkillCopiesRejectsUnmanagedNameCollision(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "source")
	destDir := filepath.Join(home, "dest")
	writeSkillDir(t, source, "source")
	writeSkillDir(t, filepath.Join(destDir, "chosen"), "user")

	_, err := syncSkillCopies([]skill.Source{{Path: source, Name: "chosen"}}, destDir, hash.DB{}, Options{})
	if err == nil || !strings.Contains(err.Error(), "not managed by chai") {
		t.Fatalf("collision error = %v", err)
	}
}

func TestSyncSkillCopiesForceStillRejectsUnmanagedNameCollision(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "source")
	destDir := filepath.Join(home, "dest")
	writeSkillDir(t, source, "source")
	writeSkillDir(t, filepath.Join(destDir, "chosen"), "user")

	_, err := syncSkillCopies([]skill.Source{{Path: source, Name: "chosen"}}, destDir, hash.DB{}, Options{Force: true})
	if err == nil || !strings.Contains(err.Error(), "not managed by chai") {
		t.Fatalf("force collision error = %v", err)
	}
}

func TestValidateUnmanagedSkillDestinationsReportsEveryCollision(t *testing.T) {
	home := t.TempDir()
	paths := []string{
		filepath.Join(home, ".claude", "skills", "one"),
		filepath.Join(home, ".agents", "skills", "one"),
		filepath.Join(home, ".cursor", "skills", "two"),
	}
	for _, path := range paths {
		writeSkillDir(t, path, "existing")
	}

	err := ValidateUnmanagedSkillDestinations([]string{"two", "one"}, home, []string{"cursor", "codex", "claude"})
	if err == nil {
		t.Fatal("expected unmanaged destination collisions")
	}
	if !strings.Contains(err.Error(), "existing skill destinations are not managed by chai") {
		t.Fatalf("collision error = %v", err)
	}
	for _, path := range paths {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("collision error missing %s: %v", path, err)
		}
	}
	sort.Strings(paths)
	if strings.Index(err.Error(), paths[0]) > strings.Index(err.Error(), paths[1]) ||
		strings.Index(err.Error(), paths[1]) > strings.Index(err.Error(), paths[2]) {
		t.Errorf("collision paths are not sorted: %v", err)
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
	skillsDir := filepath.Join(home, "dotfiles", "ai", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("reading skills: %v", err)
	}
	sources := make([]skill.Source, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			sources = append(sources, skill.Source{Name: entry.Name(), Path: filepath.Join(skillsDir, entry.Name())})
		}
	}
	output, err := captureStdout(t, func() error {
		return syncResolvedSkills(sources, home, platform.ForNames([]string{"cursor"}), Options{Force: true}, hashDB)
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

	sources := []skill.Source{
		{Name: "web-dev", Path: filepath.Join(skillsDir, "web-dev")},
		{Name: "android-dev", Path: filepath.Join(skillsDir, "android-dev")},
	}

	destDir := filepath.Join(home, ".claude", "skills")

	_, err := syncSkillCopies(sources, destDir, hashDB, Options{Force: true})
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

	source := filepath.Join(home, "dotfiles", "ai", "skills", "web-dev")
	os.MkdirAll(source, 0755)
	os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("skill web-dev"), 0644)

	if err := syncResolvedSkills([]skill.Source{{Name: "web-dev", Path: source}}, home, platform.ForNames([]string{"cursor"}), Options{Force: true}, hashDB); err != nil {
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

func TestSyncLocalSkills_UsesFrontmatterName(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "skills")
	dir := filepath.Join(root, "different-folder")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: declared-name\n---\nBody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	hashDB := hash.DB{}
	sources, err := resolveLocalSkillSources([]string{root}, home, home)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncResolvedSkills(sources, home, platform.ForNames([]string{"cursor"}), Options{Force: true}, hashDB); err != nil {
		t.Fatalf("syncing local skills: %v", err)
	}

	dest := filepath.Join(home, ".cursor", "skills", "declared-name", "SKILL.md")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("reading copied skill: %v", err)
	}
	if string(data) != content {
		t.Errorf("content = %q, want %q", data, content)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "skills", "different-folder")); !os.IsNotExist(err) {
		t.Errorf("directory name should not determine destination, got err=%v", err)
	}
}

func TestSyncLocalSkills_RemovesLastManagedSkill(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "skills")
	dir := filepath.Join(root, "skill")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: skill\n---\nBody\n"), 0644); err != nil {
		t.Fatal(err)
	}

	hashDB := hash.DB{}
	platforms := platform.ForNames([]string{"cursor"})
	sources, err := resolveLocalSkillSources([]string{root}, home, home)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncResolvedSkills(sources, home, platforms, Options{Force: true}, hashDB); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	sources, err = resolveLocalSkillSources([]string{root}, home, home)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncResolvedSkills(sources, home, platforms, Options{Force: true}, hashDB); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	dest := filepath.Join(home, ".cursor", "skills", "skill")
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("stale managed skill should be removed, got err=%v", err)
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
	if _, err := syncSkillCopies([]skill.Source{{Name: "web-dev", Path: src}}, destDir, hashDB, Options{Force: true}); err != nil {
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

	if _, err := syncSkillCopies([]skill.Source{
		{Name: "web-dev", Path: filepath.Join(skillsDir, "web-dev")},
		{Name: "old-skill", Path: filepath.Join(skillsDir, "old-skill")},
	}, destDir, hashDB, Options{Force: true}); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	if _, err := syncSkillCopies([]skill.Source{
		{Name: "web-dev", Path: filepath.Join(skillsDir, "web-dev")},
	}, destDir, hashDB, Options{Force: true}); err != nil {
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

	if _, err := syncSkillCopies([]skill.Source{{Name: "web-dev", Path: src}}, destDir, hashDB, Options{Force: true}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	userSkill := filepath.Join(destDir, "my-skill")
	os.MkdirAll(userSkill, 0755)
	os.WriteFile(filepath.Join(userSkill, "SKILL.md"), []byte("user"), 0644)

	if _, err := syncSkillCopies([]skill.Source{{Name: "web-dev", Path: src}}, destDir, hashDB, Options{Force: true}); err != nil {
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
	if _, err := syncSkillCopies([]skill.Source{{Name: "web-dev", Path: src}}, destDir, hashDB, Options{Force: true}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	firstHash := hashDB[filepath.Join(destDir, "web-dev")]

	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("v2"), 0644)
	if _, err := syncSkillCopies([]skill.Source{{Name: "web-dev", Path: src}}, destDir, hashDB, Options{Force: true}); err != nil {
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
	if _, err := syncSkillCopies([]skill.Source{{Name: "web-dev", Path: src}}, destDir, hashDB, Options{Force: true}); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	os.Remove(filepath.Join(src, "extra.md"))
	if _, err := syncSkillCopies([]skill.Source{{Name: "web-dev", Path: src}}, destDir, hashDB, Options{Force: true}); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "web-dev", "extra.md")); !os.IsNotExist(err) {
		t.Error("extra.md should have been removed when deleted from source")
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
