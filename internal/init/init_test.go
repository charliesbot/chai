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
	if !strings.Contains(tomlContent, `instructions = "~/dotfiles/ai/instructions/AGENTS.md"`) {
		t.Errorf("chai.toml missing instructions line, got:\n%s", tomlContent)
	}
	if !strings.Contains(tomlContent, `"~/dotfiles/ai/skills"`) {
		t.Errorf("chai.toml missing skills path, got:\n%s", tomlContent)
	}
	if !strings.Contains(tomlContent, `"~/dotfiles/ai/agents"`) {
		t.Errorf("chai.toml missing agents path, got:\n%s", tomlContent)
	}

	// Check AGENTS.md exists
	agentsPath := filepath.Join(home, "dotfiles", "ai", "instructions", "AGENTS.md")
	if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
		t.Error("AGENTS.md was not created")
	}
}

func TestScaffold_AbsolutePath(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(t.TempDir(), "myconfig")

	err := Scaffold(home, configDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agentsPath := filepath.Join(configDir, "instructions", "AGENTS.md")
	if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
		t.Error("AGENTS.md was not created at absolute path")
	}
}

func TestScaffold_TomlAlreadyExists(t *testing.T) {
	home := t.TempDir()

	// Create existing chai.toml
	tomlPath := filepath.Join(home, "chai.toml")
	if err := os.WriteFile(tomlPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	err := Scaffold(home, "~/dotfiles/ai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// chai.toml should not have been overwritten
	got, _ := os.ReadFile(tomlPath)
	if string(got) != "existing" {
		t.Errorf("chai.toml was overwritten, got %q", string(got))
	}
}

func TestScaffold_ExistingAgentsMD(t *testing.T) {
	home := t.TempDir()

	// Create existing AGENTS.md with custom content
	agentsDir := filepath.Join(home, "dotfiles", "ai", "instructions")
	os.MkdirAll(agentsDir, 0755)
	agentsPath := filepath.Join(agentsDir, "AGENTS.md")
	os.WriteFile(agentsPath, []byte("my custom instructions"), 0644)

	err := Scaffold(home, "~/dotfiles/ai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// AGENTS.md should not have been overwritten
	got, _ := os.ReadFile(agentsPath)
	if string(got) != "my custom instructions" {
		t.Errorf("AGENTS.md was overwritten, got %q", string(got))
	}

	// chai.toml should still have been created
	tomlPath := filepath.Join(home, "chai.toml")
	if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
		t.Error("chai.toml was not created")
	}
}
