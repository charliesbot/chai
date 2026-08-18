package githubskill

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMaterialize(t *testing.T) {
	source := t.TempDir()
	runGit(t, source, "init", "-b", "main")
	runGit(t, source, "config", "uploadpack.allowFilter", "true")
	selectedDir := "skills/weird [*?]"
	writeRepoFile(t, source, selectedDir+"/SKILL.md", "---\nname: weird-skill\n---\nBody\n")
	writeRepoFile(t, source, selectedDir+"/scripts/run.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(source, filepath.FromSlash(selectedDir), "scripts", "run.sh"), 0755); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, source, "skills/unselected/SKILL.md", "---\nname: unselected\n---\n")
	writeRepoFile(t, source, "skills/sibling.txt", "exclude")
	writeRepoFile(t, source, "README.md", "repository readme\n")
	writeRepoFile(t, source, "skills/worktrunk/SKILL.md", "---\nname: worktrunk\n---\n")
	if err := os.MkdirAll(filepath.Join(source, "skills", "worktrunk", "reference"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../README.md", filepath.Join(source, "skills", "worktrunk", "reference", "README.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeRepoFile(t, source, "skills/unsafe/SKILL.md", "---\nname: unsafe\n---\n")
	if err := os.Symlink("../../../outside.txt", filepath.Join(source, "skills", "unsafe", "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	commitRepo(t, source, "skills")
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()

	repository := filepath.Join(t.TempDir(), "repository")
	discovery, err := Discover(context.Background(), cloneURL, repository)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := Materialize(context.Background(), repository, discovery, []string{"weird-skill"})
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if selected["weird-skill"] != selectedDir {
		t.Fatalf("selected = %v", selected)
	}
	script := filepath.Join(repository, filepath.FromSlash(selectedDir), "scripts", "run.sh")
	if info, err := os.Stat(script); err != nil || info.Mode()&0100 == 0 {
		t.Fatalf("executable selected file missing or not executable: info=%v err=%v", info, err)
	}
	for _, excluded := range []string{"skills/unselected", "skills/sibling.txt", "skills/unsafe"} {
		if _, err := os.Stat(filepath.Join(repository, filepath.FromSlash(excluded))); !os.IsNotExist(err) {
			t.Errorf("unselected path %q was materialized, err=%v", excluded, err)
		}
	}

	worktrunkRepo := filepath.Join(t.TempDir(), "repository")
	worktrunkDiscovery, err := Discover(context.Background(), cloneURL, worktrunkRepo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Materialize(context.Background(), worktrunkRepo, worktrunkDiscovery, []string{"worktrunk"}); err != nil {
		t.Fatalf("Materialize worktrunk: %v", err)
	}
	reference := filepath.Join(worktrunkRepo, "skills", "worktrunk", "reference", "README.md")
	info, err := os.Lstat(reference)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("dereferenced README mode = %v, want regular file", info.Mode())
	}
	contents, err := os.ReadFile(reference)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "repository readme\n" {
		t.Fatalf("dereferenced README = %q", contents)
	}

	unsafeRepo := filepath.Join(t.TempDir(), "repository")
	unsafeDiscovery, err := Discover(context.Background(), cloneURL, unsafeRepo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Materialize(context.Background(), unsafeRepo, unsafeDiscovery, []string{"unsafe"}); err == nil || !strings.Contains(err.Error(), "escapes the repository") {
		t.Fatalf("unsafe materialization error = %v", err)
	}
}

func TestSparsePatterns(t *testing.T) {
	patterns, err := sparsePatterns([]string{"skills/weird [*?]", "!leading/hash# and space "})
	if err != nil {
		t.Fatal(err)
	}
	want := "/!leading/hash#\\ and\\ space\\ /\n/skills/weird\\ \\[\\*\\?\\]/\n"
	if string(patterns) != want {
		t.Fatalf("patterns = %q, want %q", patterns, want)
	}
	if _, err := sparsePatterns([]string{"bad\npath"}); err == nil {
		t.Fatal("newline path should be rejected")
	}
}

func TestMaterialize_RejectsAmbiguousName(t *testing.T) {
	discovery := Discovery{Candidates: []Candidate{
		{Name: "shared", Directory: "one"},
		{Name: "shared", Directory: "two"},
	}}
	if _, err := Materialize(context.Background(), t.TempDir(), discovery, []string{"shared"}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous selection error = %v", err)
	}
}

func TestMaterializeReportsEveryMissingSkill(t *testing.T) {
	discovery := Discovery{Candidates: []Candidate{{Name: "present", Directory: "skills/present"}}}
	_, err := Materialize(context.Background(), t.TempDir(), discovery, []string{"missing-two", "present", "missing-one"})
	var missing *MissingSkillsError
	if !errors.As(err, &missing) {
		t.Fatalf("error = %v, want MissingSkillsError", err)
	}
	if want := []string{"missing-one", "missing-two"}; !reflect.DeepEqual(missing.Names, want) {
		t.Fatalf("missing names = %v, want %v", missing.Names, want)
	}
}

func TestValidateSelectedEntries_RejectsSubmodule(t *testing.T) {
	entries := []TreeEntry{{Mode: "160000", Type: "commit", Path: "skill/vendor"}}
	if err := validateSelectedEntries(entries, []string{"skill"}); err == nil || !strings.Contains(err.Error(), "submodule") {
		t.Fatalf("submodule error = %v", err)
	}
}

func TestSelectedTreeSafety_RejectsTraversalAndTypeMismatch(t *testing.T) {
	if safeGitPath("../outside") {
		t.Fatal("parent traversal should be unsafe")
	}
	entries := []TreeEntry{{Mode: "100644", Type: "tree", Path: "skill/file"}}
	if err := validateSelectedEntries(entries, []string{"skill"}); err == nil || !strings.Contains(err.Error(), "non-blob") {
		t.Fatalf("mode/type mismatch error = %v", err)
	}
	repository := t.TempDir()
	if err := validateMaterializedTree(repository, "../outside"); err == nil || !strings.Contains(err.Error(), "escapes repository") {
		t.Fatalf("containment error = %v", err)
	}
}
