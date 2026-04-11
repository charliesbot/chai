package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/charliesbot/chai/internal/config"
)

func TestBuildMCPServers_ResolvesCWD(t *testing.T) {
	mcps := map[string]config.MCP{
		"context7": {
			Command: "npx",
			Args:    []string{"-y", "@upstash/context7-mcp"},
		},
		"workspace": {
			Command: "node",
			Args:    []string{"scripts/start.js"},
			CWD:     "@workspace",
		},
	}

	servers, err := buildMCPServers(mcps, "/home/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if servers["context7"].Command != "npx" {
		t.Errorf("context7.command = %q, want %q", servers["context7"].Command, "npx")
	}
	if servers["context7"].CWD != "" {
		t.Errorf("context7.cwd = %q, want empty", servers["context7"].CWD)
	}

	want := "/home/test/.chai/deps/workspace"
	if servers["workspace"].CWD != want {
		t.Errorf("workspace.cwd = %q, want %q", servers["workspace"].CWD, want)
	}
}

func TestMergeMCPIntoFile_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	servers := map[string]mcpEntry{
		"context7": {Command: "npx", Args: []string{"-y", "ctx7"}},
	}

	err := mergeMCPIntoFile(path, "mcpServers", servers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readJSON(t, path)
	mcpServers, ok := got["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers not found or wrong type in %v", got)
	}
	ctx7, ok := mcpServers["context7"].(map[string]any)
	if !ok {
		t.Fatalf("context7 not found in mcpServers")
	}
	if ctx7["command"] != "npx" {
		t.Errorf("context7.command = %v, want %q", ctx7["command"], "npx")
	}
}

func TestMergeMCPIntoFile_PreservesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Write an existing config with other keys
	existing := map[string]any{
		"globalShortcut": "Ctrl+Space",
		"mcpServers": map[string]any{
			"old-server": map[string]any{"command": "old"},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(path, data, 0644)

	servers := map[string]mcpEntry{
		"context7": {Command: "npx", Args: []string{"-y", "ctx7"}},
	}

	err := mergeMCPIntoFile(path, "mcpServers", servers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readJSON(t, path)

	// globalShortcut should be preserved
	if got["globalShortcut"] != "Ctrl+Space" {
		t.Errorf("globalShortcut = %v, want %q", got["globalShortcut"], "Ctrl+Space")
	}

	// old-server should be replaced (not merged)
	mcpServers := got["mcpServers"].(map[string]any)
	if _, ok := mcpServers["old-server"]; ok {
		t.Error("old-server should have been replaced")
	}
	if _, ok := mcpServers["context7"]; !ok {
		t.Error("context7 should be present")
	}
}

func TestMergeMCPIntoFile_EnvAndCWD(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	servers := map[string]mcpEntry{
		"gw": {
			Command: "node",
			Args:    []string{"start.js"},
			Env:     map[string]string{"API_KEY": "abc"},
			CWD:     "/path/to/workspace",
		},
	}

	err := mergeMCPIntoFile(path, "mcpServers", servers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readJSON(t, path)
	mcpServers := got["mcpServers"].(map[string]any)
	gw := mcpServers["gw"].(map[string]any)

	if gw["cwd"] != "/path/to/workspace" {
		t.Errorf("cwd = %v, want %q", gw["cwd"], "/path/to/workspace")
	}
	env := gw["env"].(map[string]any)
	if env["API_KEY"] != "abc" {
		t.Errorf("env.API_KEY = %v, want %q", env["API_KEY"], "abc")
	}
}

func TestSyncMCP_NoMCPs(t *testing.T) {
	home := t.TempDir()
	cfg := &config.Config{}

	// Should be a no-op, no error
	err := syncMCP(cfg, home, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return result
}
