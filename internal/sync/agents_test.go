package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
)

func TestSyncAgents_CopiesSubagentFiles(t *testing.T) {
	home := t.TempDir()

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte("---\nname: reviewer\n---\n"), 0644)

	hashDB := hash.DB{}
	err := syncAgents(
		[]string{"~/dotfiles/ai/subagents/*"},
		home, platform.All(), false, hashDB,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claudeAgents := filepath.Join(home, ".claude", "agents")
	info, err := os.Lstat(filepath.Join(claudeAgents, "reviewer.md"))
	if err != nil {
		t.Error("reviewer.md missing from agents dir")
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Error("reviewer.md should be a copy, not a symlink")
	}
}

func TestSyncAgents_SubagentFiles(t *testing.T) {
	home := t.TempDir()

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte("---\nname: reviewer\n---\n"), 0644)
	os.WriteFile(filepath.Join(agentsDir, "planner.md"), []byte("---\nname: planner\n---\n"), 0644)

	hashDB := hash.DB{}
	err := syncAgents(
		[]string{"~/dotfiles/ai/subagents/*"},
		home, platform.All(), false, hashDB,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
}

func TestResolveFilePatterns(t *testing.T) {
	home := t.TempDir()

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	for _, name := range []string{"reviewer.md", "planner.md", "architect.md"} {
		os.WriteFile(filepath.Join(agentsDir, name), []byte("---\nname: test\n---\n"), 0644)
	}
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
