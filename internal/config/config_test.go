package config

import (
	"os"
	"path/filepath"
	"testing"
)

const fullTOML = `
instructions = "~/dotfiles/ai/agents.md"

[deps]
workspace = "https://github.com/gemini-cli-extensions/workspace"
angular-skills = "https://github.com/angular/skills"

[skills]
paths = [
  "~/dotfiles/ai/skills/*",
  "@workspace/skills/*",
  "@angular-skills/skills/*"
]

[agents]
paths = ["~/dotfiles/ai/agents/*"]

[mcp.context7]
command = "npx"
args = ["-y", "@upstash/context7-mcp"]

[mcp.google-workspace]
command = "node"
args = ["scripts/start.js"]
cwd = "@workspace"
env = { GOOGLE_API_KEY = "key123" }
`

func TestLoad_Full(t *testing.T) {
	path := writeTempFile(t, "chai.toml", fullTOML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Instructions != "~/dotfiles/ai/agents.md" {
		t.Errorf("instructions = %q, want %q", cfg.Instructions, "~/dotfiles/ai/agents.md")
	}

	if len(cfg.Deps) != 2 {
		t.Errorf("deps count = %d, want 2", len(cfg.Deps))
	}
	if cfg.Deps["workspace"] != "https://github.com/gemini-cli-extensions/workspace" {
		t.Errorf("deps[workspace] = %q", cfg.Deps["workspace"])
	}

	if len(cfg.Skills.Paths) != 3 {
		t.Errorf("skills paths count = %d, want 3", len(cfg.Skills.Paths))
	}

	if len(cfg.Agents.Paths) != 1 {
		t.Errorf("agents paths count = %d, want 1", len(cfg.Agents.Paths))
	}

	if len(cfg.MCP) != 2 {
		t.Errorf("mcp count = %d, want 2", len(cfg.MCP))
	}

	ctx7 := cfg.MCP["context7"]
	if ctx7.Command != "npx" {
		t.Errorf("mcp[context7].command = %q, want %q", ctx7.Command, "npx")
	}
	if len(ctx7.Args) != 2 {
		t.Errorf("mcp[context7].args count = %d, want 2", len(ctx7.Args))
	}

	gw := cfg.MCP["google-workspace"]
	if gw.CWD != "@workspace" {
		t.Errorf("mcp[google-workspace].cwd = %q, want %q", gw.CWD, "@workspace")
	}
	if gw.Env["GOOGLE_API_KEY"] != "key123" {
		t.Errorf("mcp[google-workspace].env[GOOGLE_API_KEY] = %q", gw.Env["GOOGLE_API_KEY"])
	}
}

func TestLoad_MinimalConfig(t *testing.T) {
	path := writeTempFile(t, "chai.toml", `instructions = "~/agents.md"`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Instructions != "~/agents.md" {
		t.Errorf("instructions = %q, want %q", cfg.Instructions, "~/agents.md")
	}
	if len(cfg.Deps) != 0 {
		t.Errorf("deps should be empty, got %d", len(cfg.Deps))
	}
	if len(cfg.MCP) != 0 {
		t.Errorf("mcp should be empty, got %d", len(cfg.MCP))
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/chai.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if got := err.Error(); !contains(got, "config file not found") {
		t.Errorf("error = %q, want it to contain 'config file not found'", got)
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	path := writeTempFile(t, "bad.toml", `[[[broken`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
