package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/hash"
)

func TestRunWithHome_CopiesInstructions(t *testing.T) {
	home := t.TempDir()

	// Create source instructions file
	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	srcPath := filepath.Join(srcDir, "agents.md")
	content := "# My Agent Instructions\nDo good things."
	os.WriteFile(srcPath, []byte(content), 0644)

	cfg := &config.Config{
		Platforms:    []string{"claude", "gemini"},
		Instructions: "~/dotfiles/ai/agents.md",
	}

	err := RunWithHome(context.Background(), cfg, home, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Claude instructions
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("reading Claude instructions: %v", err)
	}
	if string(got) != content {
		t.Errorf("Claude instructions = %q, want %q", string(got), content)
	}

	// Verify Gemini instructions
	geminiPath := filepath.Join(home, ".gemini", "GEMINI.md")
	got, err = os.ReadFile(geminiPath)
	if err != nil {
		t.Fatalf("reading Gemini instructions: %v", err)
	}
	if string(got) != content {
		t.Errorf("Gemini instructions = %q, want %q", string(got), content)
	}
}

func TestRunWithHome_MissingInstructionsFile(t *testing.T) {
	home := t.TempDir()

	cfg := &config.Config{
		Instructions: "~/nonexistent/agents.md",
	}

	err := RunWithHome(context.Background(), cfg, home, Options{})
	if err == nil {
		t.Fatal("expected error for missing instructions file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to contain 'not found'", err.Error())
	}
}

func TestRunWithHome_EmptyInstructionsPath(t *testing.T) {
	home := t.TempDir()

	cfg := &config.Config{}

	err := RunWithHome(context.Background(), cfg, home, Options{})
	if err == nil {
		t.Fatal("expected error for empty instructions path")
	}
	if !strings.Contains(err.Error(), "no instructions path") {
		t.Errorf("error = %q, want it to contain 'no instructions path'", err.Error())
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "file.md")
	content := []byte("hello atomic")

	err := atomicWrite(path, content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(got) != "hello atomic" {
		t.Errorf("content = %q, want %q", string(got), "hello atomic")
	}

	// Verify no .tmp file left behind
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error(".tmp file was not cleaned up")
	}
}

func TestRunWithHome_DirtyDetection(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("original"), 0644)

	cfg := &config.Config{Platforms: []string{"claude", "gemini"}, Instructions: "~/dotfiles/ai/agents.md"}

	// First sync: should succeed and store hashes
	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	// Manually edit a target file
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	os.WriteFile(claudePath, []byte("manually edited"), 0644)

	// Second sync: should return DirtyError
	err := RunWithHome(context.Background(), cfg, home, Options{})
	var dirtyErr *DirtyError
	if !errors.As(err, &dirtyErr) {
		t.Fatalf("expected DirtyError, got %v", err)
	}
	if len(dirtyErr.Files) == 0 {
		t.Error("DirtyError has no files")
	}

	// With --force: should succeed
	if err := RunWithHome(context.Background(), cfg, home, Options{Force: true}); err != nil {
		t.Fatalf("force sync failed: %v", err)
	}

	// Verify overwritten
	got, _ := os.ReadFile(claudePath)
	if string(got) != "original" {
		t.Errorf("claude content = %q, want %q", string(got), "original")
	}
}

func TestRunWithHome_PromptOverwrite(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("original"), 0644)

	cfg := &config.Config{Platforms: []string{"claude", "gemini"}, Instructions: "~/dotfiles/ai/agents.md"}

	// First sync
	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	// Manually edit target
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	os.WriteFile(claudePath, []byte("edited"), 0644)

	// Sync with prompt that says yes
	alwaysYes := func(path string) (bool, error) { return true, nil }
	if err := RunWithHome(context.Background(), cfg, home, Options{Prompt: alwaysYes}); err != nil {
		t.Fatalf("prompt sync failed: %v", err)
	}

	got, _ := os.ReadFile(claudePath)
	if string(got) != "original" {
		t.Errorf("content = %q, want %q", string(got), "original")
	}
}

func TestRunWithHome_PromptSkip(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("original"), 0644)

	cfg := &config.Config{Platforms: []string{"claude", "gemini"}, Instructions: "~/dotfiles/ai/agents.md"}

	// First sync
	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	// Manually edit both targets
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	geminiPath := filepath.Join(home, ".gemini", "GEMINI.md")
	os.WriteFile(claudePath, []byte("edited"), 0644)
	os.WriteFile(geminiPath, []byte("edited"), 0644)

	// Sync with prompt that says no
	alwaysNo := func(path string) (bool, error) { return false, nil }
	if err := RunWithHome(context.Background(), cfg, home, Options{Prompt: alwaysNo}); err != nil {
		t.Fatalf("prompt sync failed: %v", err)
	}

	// Both should still have the edited content
	got, _ := os.ReadFile(claudePath)
	if string(got) != "edited" {
		t.Errorf("claude content = %q, want %q (should have been skipped)", string(got), "edited")
	}
	got, _ = os.ReadFile(geminiPath)
	if string(got) != "edited" {
		t.Errorf("gemini content = %q, want %q (should have been skipped)", string(got), "edited")
	}
}

func TestRunWithHome_CancelledContext(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("content"), 0644)

	cfg := &config.Config{Platforms: []string{"claude", "gemini"}, Instructions: "~/dotfiles/ai/agents.md"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := RunWithHome(ctx, cfg, home, Options{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !strings.Contains(err.Error(), "interrupted") {
		t.Errorf("error = %q, want it to contain 'interrupted'", err.Error())
	}

	// No files should have been written
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	if _, err := os.Stat(claudePath); !os.IsNotExist(err) {
		t.Error("CLAUDE.md should not exist after cancelled sync")
	}
}

func TestRunWithHome_SharedInstructionsDedup(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	content := "shared instructions"
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte(content), 0644)

	// Gemini and Antigravity both write to ~/.gemini/GEMINI.md.
	// The prompt should fire at most once per unique destination.
	promptCalls := 0
	cfg := &config.Config{
		Platforms:    []string{"gemini", "antigravity"},
		Instructions: "~/dotfiles/ai/agents.md",
	}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	sharedPath := filepath.Join(home, ".gemini", "GEMINI.md")
	got, err := os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("reading shared instructions: %v", err)
	}
	if string(got) != content {
		t.Errorf("shared instructions = %q, want %q", string(got), content)
	}

	// Dirty the shared file, re-sync with a counting prompt.
	os.WriteFile(sharedPath, []byte("edited"), 0644)
	countingPrompt := func(path string) (bool, error) {
		promptCalls++
		return true, nil
	}
	if err := RunWithHome(context.Background(), cfg, home, Options{Prompt: countingPrompt}); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	if promptCalls != 1 {
		t.Errorf("prompt called %d times, want 1 (shared dest should dedupe)", promptCalls)
	}

	// hashDB should have exactly one entry for the shared destination, not one per platform.
	hashDB, err := hash.Load(home)
	if err != nil {
		t.Fatalf("loading hash DB: %v", err)
	}
	if _, ok := hashDB[sharedPath]; !ok {
		t.Errorf("hash DB missing entry for %s", sharedPath)
	}
	if len(hashDB) != 1 {
		t.Errorf("hash DB has %d entries, want 1 (instructions-only sync)", len(hashDB))
	}
}

func TestRunWithHome_SharedInstructionsPromptDeclined(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("original"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"gemini", "antigravity"},
		Instructions: "~/dotfiles/ai/agents.md",
	}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	sharedPath := filepath.Join(home, ".gemini", "GEMINI.md")
	os.WriteFile(sharedPath, []byte("manually edited"), 0644)

	alwaysNo := func(path string) (bool, error) { return false, nil }
	if err := RunWithHome(context.Background(), cfg, home, Options{Prompt: alwaysNo}); err != nil {
		t.Fatalf("re-sync: %v", err)
	}

	got, _ := os.ReadFile(sharedPath)
	if string(got) != "manually edited" {
		t.Errorf("shared path = %q, want %q (prompt declined, should not overwrite)", string(got), "manually edited")
	}
}

func TestRunWithHome_OpenCodePaths(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	skillDir := filepath.Join(srcDir, "skills", "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("greet skill"), 0644)

	agentDir := filepath.Join(srcDir, "subagents")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte("reviewer body"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"opencode"},
		Instructions: "~/dotfiles/ai/agents.md",
	}
	cfg.Skills.Paths = []string{"~/dotfiles/ai/skills/*"}
	cfg.Subagents.Paths = []string{"~/dotfiles/ai/subagents/*"}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	cases := []struct {
		label string
		path  string
		body  string
	}{
		{"instructions", filepath.Join(home, ".config", "opencode", "AGENTS.md"), "hello"},
		{"skill", filepath.Join(home, ".config", "opencode", "skills", "greet", "SKILL.md"), "greet skill"},
		{"subagent", filepath.Join(home, ".config", "opencode", "agents", "reviewer.md"), "reviewer body"},
	}
	for _, c := range cases {
		got, err := os.ReadFile(c.path)
		if err != nil {
			t.Errorf("%s at %s: %v", c.label, c.path, err)
			continue
		}
		if string(got) != c.body {
			t.Errorf("%s body = %q, want %q", c.label, string(got), c.body)
		}
	}
}

func TestRunWithHome_DroidPaths(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	skillDir := filepath.Join(srcDir, "skills", "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("greet skill"), 0644)

	agentDir := filepath.Join(srcDir, "subagents")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte("reviewer body"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"droid"},
		Instructions: "~/dotfiles/ai/agents.md",
	}
	cfg.Skills.Paths = []string{"~/dotfiles/ai/skills/*"}
	cfg.Subagents.Paths = []string{"~/dotfiles/ai/subagents/*"}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	cases := []struct {
		label string
		path  string
		body  string
	}{
		{"instructions", filepath.Join(home, ".factory", "AGENTS.md"), "hello"},
		{"skill", filepath.Join(home, ".factory", "skills", "greet", "SKILL.md"), "greet skill"},
		{"droid subagent", filepath.Join(home, ".factory", "droids", "reviewer.md"), "reviewer body"},
	}
	for _, c := range cases {
		got, err := os.ReadFile(c.path)
		if err != nil {
			t.Errorf("%s at %s: %v", c.label, c.path, err)
			continue
		}
		if string(got) != c.body {
			t.Errorf("%s body = %q, want %q", c.label, string(got), c.body)
		}
	}
}

func TestRunWithHome_CodexPaths(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	skillDir := filepath.Join(srcDir, "skills", "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("greet skill"), 0644)

	agentDir := filepath.Join(srcDir, "subagents")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte("reviewer body"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"codex"},
		Instructions: "~/dotfiles/ai/agents.md",
	}
	cfg.Skills.Paths = []string{"~/dotfiles/ai/skills/*"}
	cfg.Subagents.Paths = []string{"~/dotfiles/ai/subagents/*"}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	cases := []struct {
		label string
		path  string
		body  string
	}{
		{"instructions", filepath.Join(home, ".codex", "AGENTS.md"), "hello"},
		{"skill", filepath.Join(home, ".agents", "skills", "greet", "SKILL.md"), "greet skill"},
	}
	for _, c := range cases {
		got, err := os.ReadFile(c.path)
		if err != nil {
			t.Errorf("%s at %s: %v", c.label, c.path, err)
			continue
		}
		if string(got) != c.body {
			t.Errorf("%s body = %q, want %q", c.label, string(got), c.body)
		}
	}

	// Codex has no markdown subagent target — nothing should be written under .codex/agents.
	codexAgents := filepath.Join(home, ".codex", "agents")
	if _, err := os.Stat(codexAgents); !os.IsNotExist(err) {
		t.Errorf("codex agents dir should not exist, got err=%v", err)
	}
}

func TestRunWithHome_AntigravitySkipsSubagents(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("x"), 0644)

	agentDir := filepath.Join(home, "dotfiles", "ai", "subagents")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte("reviewer body"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"gemini", "antigravity"},
		Instructions: "~/dotfiles/ai/agents.md",
	}
	cfg.Subagents.Paths = []string{"~/dotfiles/ai/subagents/*"}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Gemini gets the subagent
	geminiAgent := filepath.Join(home, ".gemini", "agents", "reviewer.md")
	if _, err := os.Stat(geminiAgent); err != nil {
		t.Errorf("gemini agent should exist: %v", err)
	}

	// Antigravity has no agents dir — nothing should be written under its skills tree
	antigravityAgents := filepath.Join(home, ".gemini", "antigravity", "agents")
	if _, err := os.Stat(antigravityAgents); !os.IsNotExist(err) {
		t.Errorf("antigravity agents dir should not exist, got err=%v", err)
	}
}

// TestRunWithHome_GeminiSkillsUseSharedAgentsDir verifies that a Gemini-only
// sync writes skills to ~/.agents/skills/ (shared, auto-discovered by Gemini),
// not the legacy ~/.gemini/skills/ path. Writing both produced "skill conflict"
// warnings on Gemini launch.
func TestRunWithHome_GeminiSkillsUseSharedAgentsDir(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	skillDir := filepath.Join(srcDir, "skills", "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("greet skill"), 0644)

	agentDir := filepath.Join(srcDir, "subagents")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte("reviewer body"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"gemini"},
		Instructions: "~/dotfiles/ai/agents.md",
	}
	cfg.Skills.Paths = []string{"~/dotfiles/ai/skills/*"}
	cfg.Subagents.Paths = []string{"~/dotfiles/ai/subagents/*"}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	sharedSkill := filepath.Join(home, ".agents", "skills", "greet", "SKILL.md")
	got, err := os.ReadFile(sharedSkill)
	if err != nil {
		t.Errorf("shared skill should exist at %s: %v", sharedSkill, err)
	} else if string(got) != "greet skill" {
		t.Errorf("shared skill body = %q, want %q", string(got), "greet skill")
	}

	geminiSkills := filepath.Join(home, ".gemini", "skills")
	if _, err := os.Stat(geminiSkills); !os.IsNotExist(err) {
		t.Errorf(".gemini/skills should not exist, got err=%v", err)
	}

	geminiAgent := filepath.Join(home, ".gemini", "agents", "reviewer.md")
	if _, err := os.Stat(geminiAgent); err != nil {
		t.Errorf("gemini agent should exist: %v", err)
	}
}

// TestRunWithHome_GeminiSkillsLeavesPreexistingAlone documents the migration
// behavior: a pre-existing ~/.gemini/skills/<name>/ from an older chai version
// is left untouched, not wiped. Users must `rm -rf ~/.gemini/skills/` themselves.
func TestRunWithHome_GeminiSkillsLeavesPreexistingAlone(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	skillDir := filepath.Join(srcDir, "skills", "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("greet skill"), 0644)

	stale := filepath.Join(home, ".gemini", "skills", "old-skill")
	os.MkdirAll(stale, 0755)
	os.WriteFile(filepath.Join(stale, "SKILL.md"), []byte("old"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"gemini"},
		Instructions: "~/dotfiles/ai/agents.md",
	}
	cfg.Skills.Paths = []string{"~/dotfiles/ai/skills/*"}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if _, err := os.Stat(filepath.Join(stale, "SKILL.md")); err != nil {
		t.Errorf("pre-existing ~/.gemini/skills/old-skill/SKILL.md should be left alone, got err=%v", err)
	}
}

func TestRunWithHome_DryRun(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("content"), 0644)

	cfg := &config.Config{Platforms: []string{"claude", "gemini"}, Instructions: "~/dotfiles/ai/agents.md"}

	err := RunWithHome(context.Background(), cfg, home, Options{DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No files should have been written
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	if _, err := os.Stat(claudePath); !os.IsNotExist(err) {
		t.Error("CLAUDE.md should not exist after dry run")
	}

	// No hash DB should exist
	hashPath := filepath.Join(home, ".chai", "hashes.json")
	if _, err := os.Stat(hashPath); !os.IsNotExist(err) {
		t.Error("hashes.json should not exist after dry run")
	}
}
