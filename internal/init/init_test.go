package init

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffold_CreatesFiles(t *testing.T) {
	home := t.TempDir()

	err := Scaffold(home, "~/dotfiles/ai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check chai.toml exists and has correct content
	tomlPath := filepath.Join(home, "chai.toml")
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatalf("reading chai.toml: %v", err)
	}
	tomlContent := string(data)
	if !strings.Contains(tomlContent, `instructions = "~/dotfiles/ai/agents.md"`) {
		t.Errorf("chai.toml missing instructions line, got:\n%s", tomlContent)
	}

	// Check agents.md exists
	agentsPath := filepath.Join(home, "dotfiles", "ai", "agents.md")
	if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
		t.Error("agents.md was not created")
	}
}

func TestScaffold_AbsolutePath(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(t.TempDir(), "myconfig")

	err := Scaffold(home, configDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agentsPath := filepath.Join(configDir, "agents.md")
	if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
		t.Error("agents.md was not created at absolute path")
	}
}

func TestScaffold_AlreadyExists(t *testing.T) {
	home := t.TempDir()

	// Create existing chai.toml
	tomlPath := filepath.Join(home, "chai.toml")
	if err := os.WriteFile(tomlPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	err := Scaffold(home, "~/dotfiles/ai")
	if err == nil {
		t.Fatal("expected error when chai.toml already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want it to contain 'already exists'", err.Error())
	}
}
