package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	toml "github.com/pelletier/go-toml/v2"
)

func TestSyncAgents_CopiesSubagentFiles(t *testing.T) {
	home := t.TempDir()

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte(`---
name: reviewer
description: Reviews code.
---
You review code.
`), 0644)

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
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte(`---
name: reviewer
description: Reviews code.
---
You review code.
`), 0644)
	os.WriteFile(filepath.Join(agentsDir, "planner.md"), []byte(`---
name: planner
description: Plans work.
---
You plan work.
`), 0644)

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

func TestSyncAgents_CopiesCursorSubagentFiles(t *testing.T) {
	home := t.TempDir()

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte(`---
name: reviewer
description: Reviews code.
---
You review code.
`), 0644)

	hashDB := hash.DB{}
	if err := syncAgents(
		[]string{"~/dotfiles/ai/subagents/*"},
		home, platform.ForNames([]string{"cursor"}), false, hashDB,
	); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(home, ".cursor", "agents", "reviewer.md")
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("reviewer.md missing from cursor agents dir: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("reviewer.md should be a copy, not a symlink")
	}
	if _, ok := hashDB[path]; !ok {
		t.Fatal("cursor agent hash was not recorded")
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

func TestSyncAgents_WritesCodexTOML(t *testing.T) {
	home := t.TempDir()

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte(`---
name: reviewer
description: Reviews implementation against the plan.
---
You are a code reviewer.
`), 0644)

	hashDB := hash.DB{}
	err := syncAgents(
		[]string{"~/dotfiles/ai/subagents/*"},
		home, platform.ForNames([]string{"codex"}), false, hashDB,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(home, ".codex", "agents", "reviewer.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading codex agent: %v", err)
	}
	var got codexAgent
	if err := toml.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing codex agent TOML: %v\n%s", err, data)
	}
	if got.Name != "reviewer" {
		t.Errorf("name = %q, want reviewer", got.Name)
	}
	if got.Description != "Reviews implementation against the plan." {
		t.Errorf("description = %q", got.Description)
	}
	if got.DeveloperInstructions != "You are a code reviewer." {
		t.Errorf("developer_instructions = %q", got.DeveloperInstructions)
	}
	if _, ok := hashDB[path]; !ok {
		t.Error("codex agent hash was not recorded")
	}
}

func TestCompileCodexAgent_SupportsCRLFFrontmatter(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "reviewer.md")
	os.WriteFile(path, []byte("---\r\nname: reviewer\r\ndescription: Reviews code.\r\n---\r\nYou review code.\r\n"), 0644)

	data, err := compileCodexAgent(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got codexAgent
	if err := toml.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing codex agent TOML: %v\n%s", err, data)
	}
	if got.Name != "reviewer" || got.Description != "Reviews code." || got.DeveloperInstructions != "You review code." {
		t.Fatalf("codex agent = %#v", got)
	}
}

func TestSyncAgents_CodexValidationErrorDoesNotWrite(t *testing.T) {
	home := t.TempDir()

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte(`---
name: reviewer
---
You are a code reviewer.
`), 0644)

	err := syncAgents(
		[]string{"~/dotfiles/ai/subagents/*"},
		home, platform.ForNames([]string{"codex"}), false, hash.DB{},
	)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "missing required frontmatter field: description") {
		t.Fatalf("error = %q", err.Error())
	}

	path := filepath.Join(home, ".codex", "agents", "reviewer.toml")
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("codex agent should not be written, stat err=%v", statErr)
	}
}

func TestSyncAgents_CodexValidationErrorDoesNotPartiallyWrite(t *testing.T) {
	home := t.TempDir()

	agentsDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "reviewer.md"), []byte(`---
name: reviewer
description: Reviews code.
---
You are a code reviewer.
`), 0644)
	os.WriteFile(filepath.Join(agentsDir, "planner.md"), []byte(`---
name: planner
description: Plans work.
---
`), 0644)

	err := syncAgents(
		[]string{"~/dotfiles/ai/subagents/*"},
		home, platform.ForNames([]string{"codex"}), false, hash.DB{},
	)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "missing required body for developer_instructions") {
		t.Fatalf("error = %q", err.Error())
	}

	destDir := filepath.Join(home, ".codex", "agents")
	if _, statErr := os.Stat(filepath.Join(destDir, "reviewer.toml")); !os.IsNotExist(statErr) {
		t.Fatalf("valid codex agent should not be written after another agent fails, stat err=%v", statErr)
	}
}
