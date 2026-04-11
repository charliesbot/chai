package sync

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliesbot/chai/internal/config"
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
		Instructions: "~/dotfiles/ai/agents.md",
	}

	err := RunWithHome(cfg, home, Options{})
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

	err := RunWithHome(cfg, home, Options{})
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

	err := RunWithHome(cfg, home, Options{})
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

	cfg := &config.Config{Instructions: "~/dotfiles/ai/agents.md"}

	// First sync: should succeed and store hashes
	if err := RunWithHome(cfg, home, Options{}); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	// Manually edit a target file
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	os.WriteFile(claudePath, []byte("manually edited"), 0644)

	// Second sync: should return DirtyError
	err := RunWithHome(cfg, home, Options{})
	var dirtyErr *DirtyError
	if !errors.As(err, &dirtyErr) {
		t.Fatalf("expected DirtyError, got %v", err)
	}
	if len(dirtyErr.Files) == 0 {
		t.Error("DirtyError has no files")
	}

	// With --force: should succeed
	if err := RunWithHome(cfg, home, Options{Force: true}); err != nil {
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

	cfg := &config.Config{Instructions: "~/dotfiles/ai/agents.md"}

	// First sync
	if err := RunWithHome(cfg, home, Options{}); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	// Manually edit target
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	os.WriteFile(claudePath, []byte("edited"), 0644)

	// Sync with prompt that says yes
	alwaysYes := func(path string) (bool, error) { return true, nil }
	if err := RunWithHome(cfg, home, Options{Prompt: alwaysYes}); err != nil {
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

	cfg := &config.Config{Instructions: "~/dotfiles/ai/agents.md"}

	// First sync
	if err := RunWithHome(cfg, home, Options{}); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	// Manually edit both targets
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	geminiPath := filepath.Join(home, ".gemini", "GEMINI.md")
	os.WriteFile(claudePath, []byte("edited"), 0644)
	os.WriteFile(geminiPath, []byte("edited"), 0644)

	// Sync with prompt that says no
	alwaysNo := func(path string) (bool, error) { return false, nil }
	if err := RunWithHome(cfg, home, Options{Prompt: alwaysNo}); err != nil {
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
