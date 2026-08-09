package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const fullTOML = `
platforms = ["claude", "opencode"]
instructions = ["~/dotfiles/ai/agents.md"]

[deps]
angular-skills = "https://github.com/angular/skills"

[deps.workspace]
url = "https://github.com/gemini-cli-extensions/workspace"
build = "npm install"

[skills]
local = ["~/dotfiles/ai/skills"]

[[skills.github]]
url = "https://github.com/vercel-labs/agent-skills"
include = ["frontend-design", "source-driven-development"]

[subagents]
paths = ["~/dotfiles/ai/subagents/*"]

[mcp.context7]
command = "npx"
args = ["-y", "@upstash/context7-mcp"]

[mcp.google-workspace]
command = "node"
args = ["scripts/start.js"]
cwd = "@workspace"
env = { GOOGLE_API_KEY = "key123" }

[antigravity.plugins]
workspace = "https://github.com/gemini-cli-extensions/workspace"
`

func TestLoad_Full(t *testing.T) {
	path := writeTempFile(t, "chai.toml", fullTOML)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Instructions) != 1 || cfg.Instructions[0] != "~/dotfiles/ai/agents.md" {
		t.Errorf("instructions = %v, want [\"~/dotfiles/ai/agents.md\"]", cfg.Instructions)
	}

	if len(cfg.Deps) != 2 {
		t.Errorf("deps count = %d, want 2", len(cfg.Deps))
	}
	ws := cfg.Deps["workspace"]
	if ws.URL != "https://github.com/gemini-cli-extensions/workspace" {
		t.Errorf("deps[workspace].url = %q", ws.URL)
	}
	if ws.Build != "npm install" {
		t.Errorf("deps[workspace].build = %q, want %q", ws.Build, "npm install")
	}
	as := cfg.Deps["angular-skills"]
	if as.URL != "https://github.com/angular/skills" {
		t.Errorf("deps[angular-skills].url = %q", as.URL)
	}
	if as.Build != "" {
		t.Errorf("deps[angular-skills].build = %q, want empty", as.Build)
	}

	if len(cfg.Skills.Local) != 1 || cfg.Skills.Local[0] != "~/dotfiles/ai/skills" {
		t.Errorf("skills local = %v, want [\"~/dotfiles/ai/skills\"]", cfg.Skills.Local)
	}
	if len(cfg.Skills.GitHub) != 1 {
		t.Fatalf("skills github count = %d, want 1", len(cfg.Skills.GitHub))
	}
	github := cfg.Skills.GitHub[0]
	if github.URL != "https://github.com/vercel-labs/agent-skills" {
		t.Errorf("skills github URL = %q", github.URL)
	}
	if len(github.Include) != 2 || github.Include[0] != "frontend-design" || github.Include[1] != "source-driven-development" {
		t.Errorf("skills github include = %v", github.Include)
	}

	if len(cfg.Subagents.Paths) != 1 {
		t.Errorf("agents paths count = %d, want 1", len(cfg.Subagents.Paths))
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

	if len(cfg.Antigravity.Plugins) != 1 {
		t.Errorf("antigravity plugins count = %d, want 1", len(cfg.Antigravity.Plugins))
	}
	if cfg.Antigravity.Plugins["workspace"] != "https://github.com/gemini-cli-extensions/workspace" {
		t.Errorf("antigravity.plugins[workspace] = %q", cfg.Antigravity.Plugins["workspace"])
	}
}

func TestSaveAtomicRoundTripsCompleteConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chai.toml")
	cfg := &Config{
		Platforms:    []string{"cursor"},
		Instructions: []string{"~/AGENTS.md"},
		Deps: map[string]Dep{
			"simple": {URL: "https://example.com/simple"},
			"built":  {URL: "https://example.com/built", Build: "make"},
		},
		Skills: Skills{
			Local:  []string{"~/skills"},
			GitHub: []GitHubSkills{{URL: "https://github.com/example/skills", Include: []string{"one"}}},
		},
		Subagents: Subagents{Paths: []string{"~/agents/*"}},
		MCP:       map[string]MCP{"ctx": {Command: "npx", Args: []string{"ctx"}}},
	}
	if err := SaveAtomic(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", loaded, cfg)
	}
}

func TestSaveAtomicUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chai.toml")
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveAtomic(path, &Config{Platforms: []string{"cursor"}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("manifest mode = %o, want 600", info.Mode().Perm())
	}
}

func TestLoadRejectsDependencyBackedLocalSkill(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chai.toml")
	if err := os.WriteFile(path, []byte(`platforms = ["cursor"]
[skills]
local = ["@dep/skills"]`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "local path") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoad_MinimalConfig(t *testing.T) {
	path := writeTempFile(t, "chai.toml", `platforms = ["cursor"]`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Platforms) != 1 || cfg.Platforms[0] != "cursor" {
		t.Errorf("platforms = %v, want [\"cursor\"]", cfg.Platforms)
	}
	if len(cfg.Deps) != 0 {
		t.Errorf("deps should be empty, got %d", len(cfg.Deps))
	}
	if len(cfg.MCP) != 0 {
		t.Errorf("mcp should be empty, got %d", len(cfg.MCP))
	}
}

func TestLoad_InstructionsArray(t *testing.T) {
	path := writeTempFile(t, "chai.toml", `
platforms = ["claude"]
instructions = ["~/core.md", "~/git.md", "@dep/rules/*.md"]
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"~/core.md", "~/git.md", "@dep/rules/*.md"}
	if len(cfg.Instructions) != len(want) {
		t.Fatalf("instructions len = %d, want %d", len(cfg.Instructions), len(want))
	}
	for i, p := range want {
		if cfg.Instructions[i] != p {
			t.Errorf("instructions[%d] = %q, want %q", i, cfg.Instructions[i], p)
		}
	}
}

func TestLoad_InvalidInstructions(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "single string",
			content: `instructions = "~/agents.md"`,
		},
		{
			name:    "integer",
			content: `instructions = 123`,
		},
		{
			name:    "non-string array item",
			content: `instructions = ["~/core.md", 123]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, "chai.toml", "platforms = [\"claude\"]\n"+tt.content)
			_, err := Load(path)
			if err == nil {
				t.Errorf("expected error for invalid instructions format in %s", tt.name)
			}
		})
	}
}

func TestLoad_RejectsInvalidManifest(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "missing platforms",
			content: `instructions = ["~/agents.md"]`,
			want:    "platforms must contain at least one platform",
		},
		{
			name:    "unsupported platform",
			content: `platforms = ["vim"]`,
			want:    `unsupported platform "vim"`,
		},
		{
			name:    "duplicate platform",
			content: `platforms = ["claude", "Claude"]`,
			want:    `duplicate platform "Claude"`,
		},
		{
			name:    "unknown root field",
			content: "platforms = [\"claude\"]\nunknown = true",
			want:    "missing field",
		},
		{
			name: "unknown nested field",
			content: `platforms = ["claude"]
[mcp.test]
command = "test"
unknown = true`,
			want: "missing field",
		},
		{
			name: "unknown dependency field",
			content: `platforms = ["claude"]
[deps.test]
url = "https://github.com/owner/repo"
unknown = true`,
			want: `dep "test": unknown field "unknown"`,
		},
		{
			name: "legacy skills paths",
			content: `platforms = ["claude"]
[skills]
paths = ["~/skills/*"]`,
			want: "missing field",
		},
		{
			name: "empty local path",
			content: `platforms = ["claude"]
[skills]
local = [""]`,
			want: "skills.local[0] must not be empty",
		},
		{
			name: "local glob",
			content: `platforms = ["claude"]
[skills]
local = ["~/skills/*"]`,
			want: "must not contain glob metacharacters",
		},
		{
			name: "duplicate local path",
			content: `platforms = ["claude"]
[skills]
local = ["~/skills", "~/skills/"]`,
			want: `duplicate local path "~/skills/"`,
		},
		{
			name: "noncanonical github URL",
			content: `platforms = ["claude"]
[[skills.github]]
url = "https://github.com/Owner/Repo.git"
include = ["skill"]`,
			want: "must be a canonical",
		},
		{
			name: "empty github include",
			content: `platforms = ["claude"]
[[skills.github]]
url = "https://github.com/owner/repo"
include = []`,
			want: "include must contain at least one skill name",
		},
		{
			name: "invalid skill name",
			content: `platforms = ["claude"]
[[skills.github]]
url = "https://github.com/owner/repo"
include = ["Bad_Name"]`,
			want: `invalid skill name "Bad_Name"`,
		},
		{
			name: "duplicate github repository",
			content: `platforms = ["claude"]
[[skills.github]]
url = "https://github.com/owner/repo"
include = ["one"]
[[skills.github]]
url = "https://github.com/owner/repo"
include = ["two"]`,
			want: `duplicate GitHub repository "https://github.com/owner/repo"`,
		},
		{
			name: "duplicate skill within repository",
			content: `platforms = ["claude"]
[[skills.github]]
url = "https://github.com/owner/repo"
include = ["one", "one"]`,
			want: `duplicate skill name "one"`,
		},
		{
			name: "skill selected from multiple repositories",
			content: `platforms = ["claude"]
[[skills.github]]
url = "https://github.com/owner/one"
include = ["shared"]
[[skills.github]]
url = "https://github.com/owner/two"
include = ["shared"]`,
			want: `skill name "shared" is selected from multiple repositories`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, "chai.toml", tt.content)
			_, err := Load(path)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
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
