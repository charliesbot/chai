package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/ui"
	toml "github.com/pelletier/go-toml/v2"
)

const greetSkillContent = "---\nname: greet\n---\ngreet skill"

func TestRunWithHome_CopiesInstructions(t *testing.T) {
	home := t.TempDir()

	// Create source instructions file
	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	srcPath := filepath.Join(srcDir, "agents.md")
	content := "# My Agent Instructions\nDo good things."
	os.WriteFile(srcPath, []byte(content), 0644)

	cfg := &config.Config{
		Platforms:    []string{"claude", "antigravity"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
	}

	err := RunWithHome(context.Background(), cfg, home, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	got, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("reading Claude instructions: %v", err)
	}
	if string(got) != content {
		t.Errorf("Claude instructions = %q, want %q", string(got), content)
	}

	antigravityPath := filepath.Join(home, ".gemini", "GEMINI.md")
	got, err = os.ReadFile(antigravityPath)
	if err != nil {
		t.Fatalf("reading Antigravity instructions: %v", err)
	}
	if string(got) != content {
		t.Errorf("Antigravity instructions = %q, want %q", string(got), content)
	}
}

func TestRunWithHome_ReportsInstructionChanges(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(previousProfile) })

	home := t.TempDir()
	srcDir := filepath.Join(home, "dotfiles", "ai")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("creating source directory: %v", err)
	}
	srcPath := filepath.Join(srcDir, "agents.md")
	if err := os.WriteFile(srcPath, []byte("v1"), 0644); err != nil {
		t.Fatalf("writing instructions: %v", err)
	}
	cfg := &config.Config{
		Platforms:    []string{"antigravity"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
	}

	first := runSyncWithOutput(t, cfg, home, Options{})
	assertOutputContains(t, first,
		"instructions",
		"\x1b[38;5;42m1 added\x1b[0m",
		"\x1b[38;5;42m+ agents.md\x1b[0m",
	)
	if strings.Contains(first, "3 added") {
		t.Fatalf("shared Antigravity destination should count once:\n%s", first)
	}

	second := runSyncWithOutput(t, cfg, home, Options{})
	assertOutputContains(t, second, "instructions", "\x1b[38;5;241m1 unchanged\x1b[0m")
	if strings.Contains(second, "+ agents.md") || strings.Contains(second, "~ agents.md") {
		t.Fatalf("unchanged instructions should not have a detail line:\n%s", second)
	}

	if err := os.WriteFile(srcPath, []byte("v2"), 0644); err != nil {
		t.Fatalf("updating instructions: %v", err)
	}
	third := runSyncWithOutput(t, cfg, home, Options{})
	assertOutputContains(t, third,
		"instructions",
		"\x1b[38;5;220m1 updated\x1b[0m",
		"\x1b[38;5;220m~ agents.md\x1b[0m",
	)

	geminiPath := filepath.Join(home, ".gemini", "GEMINI.md")
	if err := os.Remove(geminiPath); err != nil {
		t.Fatalf("removing managed target: %v", err)
	}
	recreated := runSyncWithOutput(t, cfg, home, Options{})
	assertOutputContains(t, recreated, "1 updated", "~ agents.md")
}

func TestRunWithHome_ReportsUpdatedInstructionsWithSkippedTarget(t *testing.T) {
	home := t.TempDir()
	srcDir := filepath.Join(home, "dotfiles", "ai")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("creating source directory: %v", err)
	}
	srcPath := filepath.Join(srcDir, "agents.md")
	if err := os.WriteFile(srcPath, []byte("v1"), 0644); err != nil {
		t.Fatalf("writing instructions: %v", err)
	}
	cfg := &config.Config{
		Platforms:    []string{"claude", "antigravity"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
	}
	runSyncWithOutput(t, cfg, home, Options{})

	if err := os.WriteFile(srcPath, []byte("v2"), 0644); err != nil {
		t.Fatalf("updating instructions: %v", err)
	}
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte("manual edit"), 0644); err != nil {
		t.Fatalf("editing Claude target: %v", err)
	}

	output, err := runSyncWithOutputResult(t, cfg, home, Options{
		Prompt: func(string) (bool, error) { return false, nil },
	})
	if err == nil {
		t.Fatal("expected incomplete sync error")
	}
	assertOutputContains(t, output, "! instructions", "1 updated", "1 target skipped", "~ agents.md")

	if got, _ := os.ReadFile(claudePath); string(got) != "manual edit" {
		t.Fatalf("Claude target = %q, want preserved manual edit", got)
	}
	geminiPath := filepath.Join(home, ".gemini", "GEMINI.md")
	if got, _ := os.ReadFile(geminiPath); string(got) != "v2" {
		t.Fatalf("Gemini target = %q, want v2", got)
	}
}

func runSyncWithOutput(t *testing.T, cfg *config.Config, home string, opts Options) string {
	t.Helper()
	output, err := runSyncWithOutputResult(t, cfg, home, opts)
	if err != nil {
		t.Fatalf("syncing: %v", err)
	}
	return output
}

func runSyncWithOutputResult(t *testing.T, cfg *config.Config, home string, opts Options) (string, error) {
	t.Helper()
	return captureStdout(t, func() error {
		return RunWithHome(context.Background(), cfg, home, opts)
	})
}

func TestRunWithHome_MissingInstructionsFile(t *testing.T) {
	home := t.TempDir()

	cfg := &config.Config{
		Platforms:    []string{"claude"},
		Instructions: []string{"~/nonexistent/agents.md"},
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

	cfg := &config.Config{Platforms: []string{"claude"}}

	err := RunWithHome(context.Background(), cfg, home, Options{})
	if err == nil {
		t.Fatal("expected error for empty instructions path")
	}
	if !strings.Contains(err.Error(), "no instructions path") {
		t.Errorf("error = %q, want it to contain 'no instructions path'", err.Error())
	}
}

func TestRunWithHome_CursorOnlyDoesNotRequireInstructions(t *testing.T) {
	home := t.TempDir()
	cfg := &config.Config{Platforms: []string{"cursor"}}

	output := runSyncWithOutput(t, cfg, home, Options{})
	if strings.Contains(output, "instructions") {
		t.Fatalf("cursor-only sync should omit instructions output:\n%s", output)
	}

	if _, err := os.Stat(filepath.Join(home, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("cursor-only sync should not write instructions to home, stat err=%v", err)
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

	cfg := &config.Config{Platforms: []string{"claude", "antigravity"}, Instructions: []string{"~/dotfiles/ai/agents.md"}}

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

	cfg := &config.Config{Platforms: []string{"claude", "antigravity"}, Instructions: []string{"~/dotfiles/ai/agents.md"}}

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

	cfg := &config.Config{Platforms: []string{"claude", "antigravity"}, Instructions: []string{"~/dotfiles/ai/agents.md"}}

	// First sync
	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	// Manually edit both targets
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	antigravityPath := filepath.Join(home, ".gemini", "GEMINI.md")
	os.WriteFile(claudePath, []byte("edited"), 0644)
	os.WriteFile(antigravityPath, []byte("edited"), 0644)

	// Sync with prompt that says no
	alwaysNo := func(path string) (bool, error) { return false, nil }
	output, err := runSyncWithOutputResult(t, cfg, home, Options{Prompt: alwaysNo})
	if err == nil {
		t.Fatal("expected incomplete sync error")
	}
	assertOutputContains(t, output, "! instructions", "2 targets skipped")

	// Both should still have the edited content
	got, _ := os.ReadFile(claudePath)
	if string(got) != "edited" {
		t.Errorf("claude content = %q, want %q (should have been skipped)", string(got), "edited")
	}
	got, _ = os.ReadFile(antigravityPath)
	if string(got) != "edited" {
		t.Errorf("antigravity content = %q, want %q (should have been skipped)", string(got), "edited")
	}
}

func TestRunWithHome_CancelledContext(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("content"), 0644)

	cfg := &config.Config{Platforms: []string{"claude", "antigravity"}, Instructions: []string{"~/dotfiles/ai/agents.md"}}

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

	// All Antigravity targets write to ~/.gemini/GEMINI.md.
	// The prompt should fire at most once per unique destination.
	promptCalls := 0
	cfg := &config.Config{
		Platforms:    []string{"antigravity"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
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

func TestPlatformStatus_CollapsesAntigravityTargets(t *testing.T) {
	status := newPlatformStatus(platform.ForNames([]string{"claude", "antigravity", "opencode"}))

	got := status.statuses()
	if len(got) != 3 {
		t.Fatalf("statuses count = %d, want 3: %#v", len(got), got)
	}
	if got[0].Name != "Claude" || got[1].Name != "Antigravity" || got[2].Name != "OpenCode" {
		t.Fatalf("statuses = %#v, want Claude, Antigravity, OpenCode", got)
	}

	status.setFailed("Antigravity CLI")
	got = status.statuses()
	if got[1].State != ui.PlatformFailed {
		t.Fatalf("Antigravity aggregate state = %v, want failed", got[1].State)
	}
}

func TestRunWithHome_SharedInstructionsPromptDeclined(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("original"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"antigravity"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
	}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	sharedPath := filepath.Join(home, ".gemini", "GEMINI.md")
	os.WriteFile(sharedPath, []byte("manually edited"), 0644)

	alwaysNo := func(path string) (bool, error) { return false, nil }
	if err := RunWithHome(context.Background(), cfg, home, Options{Prompt: alwaysNo}); err == nil {
		t.Fatal("expected incomplete sync error")
	}

	got, _ := os.ReadFile(sharedPath)
	if string(got) != "manually edited" {
		t.Errorf("shared path = %q, want %q (prompt declined, should not overwrite)", string(got), "manually edited")
	}
}

func TestRunWithHome_AntigravityPaths(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	skillDir := filepath.Join(srcDir, "skills", "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(greetSkillContent), 0644)

	agentDir := filepath.Join(srcDir, "subagents")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte("reviewer body"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"antigravity"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
	}
	cfg.Skills.Local = []string{"~/dotfiles/ai/skills"}
	cfg.Subagents.Paths = []string{"~/dotfiles/ai/subagents/*"}
	cfg.MCP = map[string]config.MCP{
		"context7": {Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
	}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	cases := []struct {
		label string
		path  string
		body  string
	}{
		{"instructions", filepath.Join(home, ".gemini", "GEMINI.md"), "hello"},
		{"ide skill", filepath.Join(home, ".gemini", "antigravity-ide", "skills", "greet", "SKILL.md"), greetSkillContent},
		{"legacy skill", filepath.Join(home, ".gemini", "antigravity", "skills", "greet", "SKILL.md"), greetSkillContent},
		{"skill", filepath.Join(home, ".gemini", "antigravity-cli", "skills", "greet", "SKILL.md"), greetSkillContent},
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

	mcpPath := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	mcp := readJSON(t, mcpPath)
	servers, ok := mcp["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers missing or wrong type in %s: %#v", mcpPath, mcp["mcpServers"])
	}
	ctx7, ok := servers["context7"].(map[string]any)
	if !ok {
		t.Fatalf("context7 entry missing in %s", mcpPath)
	}
	if ctx7["command"] != "npx" {
		t.Errorf("%s context7.command = %v, want npx", mcpPath, ctx7["command"])
	}

	for _, obsoletePath := range []string{
		filepath.Join(home, ".gemini", "antigravity-ide", "mcp_config.json"),
		filepath.Join(home, ".gemini", "antigravity", "mcp_config.json"),
		filepath.Join(home, ".gemini", "antigravity-cli", "mcp_config.json"),
	} {
		if _, err := os.Stat(obsoletePath); !os.IsNotExist(err) {
			t.Errorf("obsolete MCP config %s should not exist, got err=%v", obsoletePath, err)
		}
	}

	// Antigravity has no standalone subagents dir. Nothing should be written
	// under any agents/ path for these targets.
	for _, agentsDir := range []string{
		filepath.Join(home, ".gemini", "antigravity-ide", "agents"),
		filepath.Join(home, ".gemini", "antigravity", "agents"),
		filepath.Join(home, ".gemini", "antigravity-cli", "agents"),
	} {
		if _, err := os.Stat(agentsDir); !os.IsNotExist(err) {
			t.Errorf("%s should not exist, got err=%v", agentsDir, err)
		}
	}
}

func TestRunWithHome_InvalidLocalSkillDoesNotWriteInstructions(t *testing.T) {
	home := t.TempDir()
	srcDir := filepath.Join(home, "sources")
	if err := os.MkdirAll(filepath.Join(srcDir, "skills", "broken"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "AGENTS.md"), []byte("instructions"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "skills", "broken", "SKILL.md"), []byte("missing frontmatter"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Platforms:    []string{"claude"},
		Instructions: []string{filepath.Join(srcDir, "AGENTS.md")},
		Skills:       config.Skills{Local: []string{filepath.Join(srcDir, "skills")}},
	}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err == nil {
		t.Fatal("expected invalid local skill error")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "CLAUDE.md")); !os.IsNotExist(err) {
		t.Errorf("instructions changed before skill validation completed, got err=%v", err)
	}
}

func TestRunWithHome_SyncsCachedGitHubSkills(t *testing.T) {
	home := t.TempDir()
	id, _ := githubskill.ParseCanonical("https://github.com/example/skills")
	cache := githubskill.CacheDir(home, id)
	repository := githubskill.RepositoryDir(cache)
	writeSkillDir(t, filepath.Join(repository, "moved", "chosen"), "---\nname: chosen\n---\nremote")
	if err := githubskill.CompleteStaging(cache, id, map[string]string{"chosen": "moved/chosen"}, "abc123"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Platforms: []string{"cursor"},
		Skills: config.Skills{GitHub: []config.GitHubSkills{{
			URL: id.URL(), Include: []string{"chosen"},
		}}},
	}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "skills", "chosen", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestRunWithHome_MissingGitHubCacheDoesNotWriteInstructions(t *testing.T) {
	home := t.TempDir()
	instructions := filepath.Join(home, "source.md")
	if err := os.WriteFile(instructions, []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Platforms:    []string{"claude"},
		Instructions: []string{instructions},
		Skills: config.Skills{GitHub: []config.GitHubSkills{{
			URL: "https://github.com/example/skills", Include: []string{"chosen"},
		}}},
	}

	err := RunWithHome(context.Background(), cfg, home, Options{})
	if err == nil || !strings.Contains(err.Error(), "chai update") {
		t.Fatalf("missing cache error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("instructions were written before cache validation: %v", err)
	}
}

func TestRunWithHome_RejectsLocalRemoteNameConflict(t *testing.T) {
	home := t.TempDir()
	local := filepath.Join(home, "local")
	writeSkillDir(t, local, "---\nname: local\n---\nlocal")
	id, _ := githubskill.ParseCanonical("https://github.com/example/skills")
	cache := githubskill.CacheDir(home, id)
	writeSkillDir(t, filepath.Join(githubskill.RepositoryDir(cache), "remote"), "---\nname: local\n---\nremote")
	if err := githubskill.CompleteStaging(cache, id, map[string]string{"local": "remote"}, "abc123"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Platforms: []string{"cursor"},
		Skills: config.Skills{
			Local:  []string{local},
			GitHub: []config.GitHubSkills{{URL: id.URL(), Include: []string{"local"}}},
		},
	}
	if err := RunWithHome(context.Background(), cfg, home, Options{}); err == nil || !strings.Contains(err.Error(), "duplicate skill name") {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestRunWithHome_PersistsSkillOwnershipWhenMCPWriteFails(t *testing.T) {
	home := t.TempDir()
	local := filepath.Join(home, "local")
	writeSkillDir(t, local, "---\nname: local\n---\n")
	mcpPath := filepath.Join(home, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(mcpPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mcpPath, []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Platforms: []string{"cursor"},
		Skills:    config.Skills{Local: []string{local}},
		MCP:       map[string]config.MCP{"ctx": {Command: "ctx"}},
	}
	if err := RunWithHome(context.Background(), cfg, home, Options{}); err == nil {
		t.Fatal("expected MCP write failure")
	}
	dest := filepath.Join(home, ".cursor", "skills", "local")
	db, err := hash.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, managed := db[dest]; !managed {
		t.Fatal("skill ownership was not persisted after downstream failure")
	}
	if err := os.WriteFile(mcpPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("rerun after downstream failure: %v", err)
	}
}

func TestRunWithHome_NoConfiguredSkillsRemovesManagedSkill(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, ".cursor", "skills", "old-skill")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	sum, err := dirHash(dest)
	if err != nil {
		t.Fatal(err)
	}
	if err := (hash.DB{dest: sum}).Save(home); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Platforms: []string{"cursor"}}
	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("stale managed skill should be removed, got err=%v", err)
	}
}

func TestRunWithHome_OpenCodePaths(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	skillDir := filepath.Join(srcDir, "skills", "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(greetSkillContent), 0644)

	agentDir := filepath.Join(srcDir, "subagents")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte("reviewer body"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"opencode"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
	}
	cfg.Skills.Local = []string{"~/dotfiles/ai/skills"}
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
		{"skill", filepath.Join(home, ".config", "opencode", "skills", "greet", "SKILL.md"), greetSkillContent},
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

func TestRunWithHome_PiPathsAndSkipsUnsupportedFeatures(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	skillDir := filepath.Join(srcDir, "skills", "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(greetSkillContent), 0644)

	agentDir := filepath.Join(srcDir, "subagents")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte("reviewer body"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"pi"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
		MCP: map[string]config.MCP{
			"ctx7": {Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
		},
	}
	cfg.Skills.Local = []string{"~/dotfiles/ai/skills"}
	cfg.Subagents.Paths = []string{"~/dotfiles/ai/subagents/*"}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	cases := []struct {
		label string
		path  string
		body  string
	}{
		{"instructions", filepath.Join(home, ".pi", "agent", "AGENTS.md"), "hello"},
		{"skill", filepath.Join(home, ".pi", "agent", "skills", "greet", "SKILL.md"), greetSkillContent},
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

	for _, unsupported := range []string{
		filepath.Join(home, ".pi", "agent", "agents"),
		filepath.Join(home, ".pi", "agent", "mcp.json"),
	} {
		if _, err := os.Stat(unsupported); !os.IsNotExist(err) {
			t.Errorf("unsupported Pi target %s should not exist, got err=%v", unsupported, err)
		}
	}
}

func TestRunWithHome_PiDoesNotBlockClaudeMCP(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"pi", "claude"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
		MCP: map[string]config.MCP{
			"ctx7": {Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
		},
	}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	claude := readJSON(t, filepath.Join(home, ".claude.json"))
	servers, ok := claude["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers = %#v, want object", claude["mcpServers"])
	}
	if _, ok := servers["ctx7"]; !ok {
		t.Fatalf("mcpServers = %#v, want ctx7", servers)
	}
}

func TestRunWithHome_DroidSkipsCustomModelsWhenUnconfigured(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"droid"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
	}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	settings := filepath.Join(home, ".factory", "settings.json")
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Fatalf("settings.json should not be created when Droid custom models are unconfigured, err=%v", err)
	}
}

func TestRunWithHome_DroidConfiguredCustomModels(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	settings := filepath.Join(home, ".factory", "settings.json")
	os.MkdirAll(filepath.Dir(settings), 0755)
	os.WriteFile(settings, []byte(`{"theme":"dark"}`), 0644)

	cfg := &config.Config{
		Platforms:    []string{"droid"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
		Droid: config.DroidConfig{CustomModels: []config.CustomModel{
			{
				Model:           "openai/gpt-4o-mini",
				DisplayName:     "GPT-4o Mini",
				BaseURL:         "https://api.openai.com/v1",
				APIKey:          "${OPENAI_API_KEY}",
				Provider:        "generic-chat-completion-api",
				MaxOutputTokens: 4096,
			},
		}},
	}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	got := readJSON(t, settings)
	if got["theme"] != "dark" {
		t.Fatalf("unrelated setting was not preserved: %#v", got)
	}
	models, ok := got["customModels"].([]any)
	if !ok || len(models) != 1 {
		t.Fatalf("customModels = %#v, want one configured model", got["customModels"])
	}
	model, ok := models[0].(map[string]any)
	if !ok {
		t.Fatalf("custom model has wrong type: %#v", models[0])
	}
	if model["model"] != "openai/gpt-4o-mini" {
		t.Errorf("model = %v", model["model"])
	}
	if model["displayName"] != "GPT-4o Mini" {
		t.Errorf("displayName = %v", model["displayName"])
	}
	if model["apiKey"] != "${OPENAI_API_KEY}" {
		t.Errorf("apiKey = %v", model["apiKey"])
	}
}

func TestRunWithHome_DroidPaths(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("hello"), 0644)

	skillDir := filepath.Join(srcDir, "skills", "greet")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(greetSkillContent), 0644)

	agentDir := filepath.Join(srcDir, "subagents")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte("reviewer body"), 0644)

	cfg := &config.Config{
		Platforms:    []string{"droid"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
	}
	cfg.Skills.Local = []string{"~/dotfiles/ai/skills"}
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
		{"skill", filepath.Join(home, ".factory", "skills", "greet", "SKILL.md"), greetSkillContent},
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
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(greetSkillContent), 0644)

	agentDir := filepath.Join(srcDir, "subagents")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte(`---
name: reviewer
description: Reviews code.
---
reviewer body`), 0644)

	cfg := &config.Config{
		Platforms:    []string{"codex"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
	}
	cfg.Skills.Local = []string{"~/dotfiles/ai/skills"}
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
		{"skill", filepath.Join(home, ".agents", "skills", "greet", "SKILL.md"), greetSkillContent},
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

	data, err := os.ReadFile(filepath.Join(home, ".codex", "agents", "reviewer.toml"))
	if err != nil {
		t.Fatalf("reading codex subagent: %v", err)
	}
	var got codexAgent
	if err := toml.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing codex subagent: %v\n%s", err, data)
	}
	if got.Name != "reviewer" || got.Description != "Reviews code." || got.DeveloperInstructions != "reviewer body" {
		t.Fatalf("codex subagent = %#v", got)
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
		Platforms:    []string{"antigravity"},
		Instructions: []string{"~/dotfiles/ai/agents.md"},
	}
	cfg.Subagents.Paths = []string{"~/dotfiles/ai/subagents/*"}

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Antigravity has no agents dir — nothing should be written under its target trees.
	for _, antigravityAgents := range []string{
		filepath.Join(home, ".gemini", "antigravity-ide", "agents"),
		filepath.Join(home, ".gemini", "antigravity", "agents"),
		filepath.Join(home, ".gemini", "antigravity-cli", "agents"),
	} {
		if _, err := os.Stat(antigravityAgents); !os.IsNotExist(err) {
			t.Errorf("%s should not exist, got err=%v", antigravityAgents, err)
		}
	}
}

func TestRunWithHome_DryRun(t *testing.T) {
	home := t.TempDir()

	srcDir := filepath.Join(home, "dotfiles", "ai")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "agents.md"), []byte("content"), 0644)

	cfg := &config.Config{Platforms: []string{"claude", "antigravity"}, Instructions: []string{"~/dotfiles/ai/agents.md"}}

	output := runSyncWithOutput(t, cfg, home, Options{DryRun: true})
	assertOutputContains(t, output, "instructions", "source:", "first sync")
	if strings.Contains(output, "1 added") {
		t.Fatalf("dry-run should keep detailed preview output:\n%s", output)
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
