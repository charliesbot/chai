package sync

import (
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

	err := RunWithHome(cfg, home)
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

	err := RunWithHome(cfg, home)
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

	err := RunWithHome(cfg, home)
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
