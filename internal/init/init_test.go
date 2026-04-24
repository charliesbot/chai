package init

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffold_CreatesToml(t *testing.T) {
	home := t.TempDir()

	err := Scaffold(home, "~/dotfiles/ai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tomlPath := filepath.Join(home, "chai.toml")
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		t.Fatalf("reading chai.toml: %v", err)
	}
	tomlContent := string(data)
	if !strings.Contains(tomlContent, `instructions = "~/dotfiles/ai/instructions/AGENTS.md"`) {
		t.Errorf("chai.toml missing instructions line, got:\n%s", tomlContent)
	}
	for _, p := range []string{`"claude"`, `"gemini"`, `"opencode"`} {
		if !strings.Contains(tomlContent, p) {
			t.Errorf("chai.toml missing platform %s, got:\n%s", p, tomlContent)
		}
	}
	if !strings.Contains(tomlContent, `"~/dotfiles/ai/skills"`) {
		t.Errorf("chai.toml missing skills path, got:\n%s", tomlContent)
	}
	if !strings.Contains(tomlContent, `"~/dotfiles/ai/subagents"`) {
		t.Errorf("chai.toml missing agents path, got:\n%s", tomlContent)
	}
}

func TestScaffold_DoesNotCreateDirectories(t *testing.T) {
	home := t.TempDir()

	err := Scaffold(home, "~/dotfiles/ai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dirPath := filepath.Join(home, "dotfiles", "ai")
	if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
		t.Error("Scaffold should not create directories, but the config directory exists")
	}
}

func TestScaffold_TomlAlreadyExists(t *testing.T) {
	home := t.TempDir()

	tomlPath := filepath.Join(home, "chai.toml")
	if err := os.WriteFile(tomlPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("writing file: %v", err)
	}

	err := Scaffold(home, "~/dotfiles/ai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(tomlPath)
	if string(got) != "existing" {
		t.Errorf("chai.toml was overwritten, got %q", string(got))
	}
}
