package githubskill

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscover(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
	source := t.TempDir()
	runGit(t, source, "init", "-b", "main")
	runGit(t, source, "config", "uploadpack.allowFilter", "true")
	writeRepoFile(t, source, "README.md", "first")
	commitRepo(t, source, "first")
	writeRepoFile(t, source, "plugins/folder/SKILL.md", "---\nname: declared-name\ndescription: Useful\n---\nBody\n")
	writeRepoFile(t, source, "broken/SKILL.md", "no frontmatter")
	writeRepoFile(t, source, "SKILL.md", "---\nname: root-skill\n---\n")
	writeRepoFile(t, source, "unrelated.txt", "do not materialize")
	commitRepo(t, source, "skills")

	dest := filepath.Join(t.TempDir(), "repository")
	result, err := Discover(context.Background(), (&url.URL{Scheme: "file", Path: source}).String(), dest)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].Name != "declared-name" || result.Candidates[0].Description != "Useful" || result.Candidates[0].Directory != "plugins/folder" {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if len(result.Problems) != 2 || !strings.Contains(result.Problems[0].Error(), "root-level") || !strings.Contains(result.Problems[1].Error(), "frontmatter") {
		t.Fatalf("problems = %v", result.Problems)
	}
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 1 || entries[0].Name() != ".git" {
		t.Fatalf("discovery worktree was materialized: entries=%v err=%v", entries, err)
	}
	if got := strings.TrimSpace(runGit(t, dest, "rev-list", "--count", "HEAD")); got != "1" {
		t.Fatalf("history count = %q, want 1", got)
	}
	if got := strings.TrimSpace(runGit(t, dest, "config", "--get", "remote.origin.partialclonefilter")); got != "blob:none" {
		t.Fatalf("partial clone filter = %q", got)
	}
}

func TestDiscoverRejectsFullCloneFallback(t *testing.T) {
	source := t.TempDir()
	runGit(t, source, "init", "-b", "main")
	writeRepoFile(t, source, "skill/SKILL.md", "---\nname: skill\n---\n")
	commitRepo(t, source, "skill")
	cloneURL := (&url.URL{Scheme: "file", Path: source}).String()

	_, err := Discover(context.Background(), cloneURL, filepath.Join(t.TempDir(), "repository"))
	if err == nil || !strings.Contains(err.Error(), "full clone fallback") {
		t.Fatalf("fallback error = %v", err)
	}
}

func TestParseTree_PreservesTabsInPath(t *testing.T) {
	entries, err := parseTree([]byte("100644 blob abc123\tdir/with\ttab/SKILL.md\x00"))
	if err != nil {
		t.Fatalf("parseTree: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "dir/with\ttab/SKILL.md" {
		t.Fatalf("entries = %+v", entries)
	}
}

func writeRepoFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func commitRepo(t *testing.T, root, message string) {
	t.Helper()
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "-m", message)
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
