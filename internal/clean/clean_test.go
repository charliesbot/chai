package clean

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/hash"
)

func TestRunWithHome_RemovesConfiguredPlatformOutputs(t *testing.T) {
	home := t.TempDir()
	cfg := &config.Config{Platforms: []string{"antigravity", "codex"}}

	dirs := []string{
		filepath.Join(home, ".gemini", "antigravity-ide", "skills"),
		filepath.Join(home, ".gemini", "antigravity", "skills"),
		filepath.Join(home, ".gemini", "antigravity-cli", "skills"),
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".codex", "agents"),
	}
	for _, dir := range dirs {
		writeFile(t, filepath.Join(dir, "managed", "SKILL.md"), "managed")
	}
	unselected := filepath.Join(home, ".claude", "skills", "keep", "SKILL.md")
	writeFile(t, unselected, "keep")

	db := hash.DB{
		filepath.Join(home, ".gemini", "antigravity", "skills", "managed"): "hash",
		filepath.Join(home, ".agents", "skills", "managed"):                "hash",
		filepath.Join(home, ".codex", "agents", "reviewer.toml"):           "hash",
		filepath.Join(home, ".claude", "skills", "keep"):                   "hash",
	}
	if err := db.Save(home); err != nil {
		t.Fatalf("saving hash DB: %v", err)
	}

	writeJSON(t, filepath.Join(home, ".gemini", "antigravity", "mcp_config.json"), map[string]any{
		"mcpServers": map[string]any{"ctx": map[string]any{"command": "npx"}},
		"other":      true,
	})
	writeJSON(t, filepath.Join(home, ".gemini", "antigravity-ide", "mcp_config.json"), map[string]any{
		"mcpServers": map[string]any{"ctx": map[string]any{"command": "npx"}},
		"other":      "ide",
	})
	writeTOML(t, filepath.Join(home, ".codex", "config.toml"), map[string]any{
		"mcp_servers": map[string]any{"ctx": map[string]any{"command": "npx"}},
		"model":       "gpt-5",
	})
	writeJSON(t, filepath.Join(home, ".claude.json"), map[string]any{
		"mcpServers": map[string]any{"keep": map[string]any{"command": "npx"}},
	})

	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatalf("clean: %v", err)
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("%s should have been removed, stat err=%v", dir, err)
		}
	}
	if _, err := os.Stat(unselected); err != nil {
		t.Fatalf("unselected platform output should remain: %v", err)
	}

	antigravity := readJSON(t, filepath.Join(home, ".gemini", "antigravity", "mcp_config.json"))
	if _, ok := antigravity["mcpServers"]; ok {
		t.Fatal("antigravity mcpServers should have been removed")
	}
	if antigravity["other"] != true {
		t.Fatalf("unrelated antigravity key was not preserved: %#v", antigravity)
	}

	codex := readTOML(t, filepath.Join(home, ".codex", "config.toml"))
	if _, ok := codex["mcp_servers"]; ok {
		t.Fatal("codex mcp_servers should have been removed")
	}
	if codex["model"] != "gpt-5" {
		t.Fatalf("unrelated codex key was not preserved: %#v", codex)
	}

	claude := readJSON(t, filepath.Join(home, ".claude.json"))
	if _, ok := claude["mcpServers"]; !ok {
		t.Fatal("unselected platform MCP config should remain")
	}

	gotDB, err := hash.Load(home)
	if err != nil {
		t.Fatalf("loading hash DB: %v", err)
	}
	for path := range db {
		if pathWithin(filepath.Join(home, ".claude", "skills"), path) {
			if _, ok := gotDB[path]; !ok {
				t.Fatalf("unselected hash entry should remain: %s", path)
			}
			continue
		}
		if _, ok := gotDB[path]; ok {
			t.Fatalf("hash entry should have been removed: %s", path)
		}
	}
}

func TestRunWithHome_DryRunDoesNotRemoveOutputs(t *testing.T) {
	home := t.TempDir()
	cfg := &config.Config{Platforms: []string{"codex"}}

	skillsDir := filepath.Join(home, ".agents", "skills")
	agentsDir := filepath.Join(home, ".codex", "agents")
	configPath := filepath.Join(home, ".codex", "config.toml")
	writeFile(t, filepath.Join(skillsDir, "managed", "SKILL.md"), "managed")
	writeFile(t, filepath.Join(agentsDir, "reviewer.toml"), "managed")
	writeTOML(t, configPath, map[string]any{
		"mcp_servers": map[string]any{"ctx": map[string]any{"command": "npx"}},
	})

	if err := RunWithHome(context.Background(), cfg, home, Options{DryRun: true}); err != nil {
		t.Fatalf("clean dry-run: %v", err)
	}

	for _, path := range []string{skillsDir, agentsDir, configPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s should remain after dry-run: %v", path, err)
		}
	}
	codex := readTOML(t, configPath)
	if _, ok := codex["mcp_servers"]; !ok {
		t.Fatal("mcp_servers should remain after dry-run")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("creating parent dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func writeJSON(t *testing.T, path string, value map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshaling JSON: %v", err)
	}
	writeFile(t, path, string(append(data, '\n')))
}

func writeTOML(t *testing.T, path string, value map[string]any) {
	t.Helper()
	data, err := toml.Marshal(value)
	if err != nil {
		t.Fatalf("marshaling TOML: %v", err)
	}
	writeFile(t, path, string(data))
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return out
}

func readTOML(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var out map[string]any
	if err := toml.Unmarshal(data, &out); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return out
}
