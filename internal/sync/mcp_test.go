package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/platform"
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

	servers := map[string]any{
		"context7": mcpEntry{Command: "npx", Args: []string{"-y", "ctx7"}},
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

func TestMergeMCPIntoFile_EmptyFile(t *testing.T) {
	// Antigravity creates ~/.gemini/antigravity/mcp_config.json as a zero-byte
	// file on first launch. We should treat that like a missing file.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("creating empty file: %v", err)
	}

	servers := map[string]any{
		"ctx": mcpEntry{Command: "npx", Args: []string{"ctx"}},
	}

	if err := mergeMCPIntoFile(path, "mcpServers", servers); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readJSON(t, path)
	mcpServers, ok := got["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers not found or wrong type in %v", got)
	}
	if _, ok := mcpServers["ctx"]; !ok {
		t.Error("ctx should be present")
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

	servers := map[string]any{
		"context7": mcpEntry{Command: "npx", Args: []string{"-y", "ctx7"}},
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

	servers := map[string]any{
		"gw": mcpEntry{
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

func TestBuildOpenCodeMCPServers_BundlesCommandAndArgs(t *testing.T) {
	standard := map[string]mcpEntry{
		"ctx7": {
			Command: "npx",
			Args:    []string{"-y", "@upstash/context7-mcp"},
			Env:     map[string]string{"FOO": "bar"},
			CWD:     "/ignored/by/opencode",
		},
	}

	got := buildOpenCodeMCPServers(standard)
	entry, ok := got["ctx7"]
	if !ok {
		t.Fatalf("ctx7 missing from output")
	}
	if entry.Type != "local" {
		t.Errorf("type = %q, want %q", entry.Type, "local")
	}
	wantCmd := []string{"npx", "-y", "@upstash/context7-mcp"}
	if len(entry.Command) != len(wantCmd) {
		t.Fatalf("command = %v, want %v", entry.Command, wantCmd)
	}
	for i, v := range wantCmd {
		if entry.Command[i] != v {
			t.Errorf("command[%d] = %q, want %q", i, entry.Command[i], v)
		}
	}
	if entry.Environment["FOO"] != "bar" {
		t.Errorf("environment.FOO = %q, want %q", entry.Environment["FOO"], "bar")
	}
	if !entry.Enabled {
		t.Error("enabled should default to true")
	}
}

func TestSyncMCP_WritesOpenCodeFormat(t *testing.T) {
	home := t.TempDir()

	cfg := &config.Config{
		MCP: map[string]config.MCP{
			"ctx7": {Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
		},
	}
	opencode := platform.ForNames([]string{"opencode"})
	if len(opencode) != 1 {
		t.Fatalf("expected one platform match for opencode, got %d", len(opencode))
	}

	if err := syncMCP(cfg, home, opencode, false); err != nil {
		t.Fatalf("syncMCP: %v", err)
	}

	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	got := readJSON(t, path)
	mcp, ok := got["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("mcp key missing or wrong type in %v", got)
	}
	if _, ok := got["mcpServers"]; ok {
		t.Error("mcpServers should not be written for OpenCode")
	}

	entry, ok := mcp["ctx7"].(map[string]any)
	if !ok {
		t.Fatalf("ctx7 missing from mcp")
	}
	if entry["type"] != "local" {
		t.Errorf("type = %v, want %q", entry["type"], "local")
	}
	cmd, ok := entry["command"].([]any)
	if !ok {
		t.Fatalf("command = %v, want array", entry["command"])
	}
	want := []string{"npx", "-y", "@upstash/context7-mcp"}
	if len(cmd) != len(want) {
		t.Fatalf("command length = %d, want %d", len(cmd), len(want))
	}
	for i, v := range want {
		if cmd[i] != v {
			t.Errorf("command[%d] = %v, want %q", i, cmd[i], v)
		}
	}
	if entry["enabled"] != true {
		t.Errorf("enabled = %v, want true", entry["enabled"])
	}
	for _, forbidden := range []string{"cwd", "args"} {
		if _, ok := entry[forbidden]; ok {
			t.Errorf("OpenCode entry should not contain %q field, got %v", forbidden, entry[forbidden])
		}
	}
}

func TestBuildCodexMCPServers_DropsCWD(t *testing.T) {
	standard := map[string]mcpEntry{
		"pencil": {
			Command: "/Applications/Pencil.app/mcp",
			Args:    []string{"--app", "desktop"},
			Env:     map[string]string{"FOO": "bar"},
			CWD:     "/ignored/by/codex",
		},
	}

	got := buildCodexMCPServers(standard)
	entry, ok := got["pencil"]
	if !ok {
		t.Fatalf("pencil missing from output")
	}
	if entry.Command != "/Applications/Pencil.app/mcp" {
		t.Errorf("command = %q", entry.Command)
	}
	wantArgs := []string{"--app", "desktop"}
	if len(entry.Args) != len(wantArgs) {
		t.Fatalf("args = %v, want %v", entry.Args, wantArgs)
	}
	for i, v := range wantArgs {
		if entry.Args[i] != v {
			t.Errorf("args[%d] = %q, want %q", i, entry.Args[i], v)
		}
	}
	if entry.Env["FOO"] != "bar" {
		t.Errorf("env.FOO = %q", entry.Env["FOO"])
	}
}

func TestMergeMCPIntoTOMLFile_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	servers := map[string]any{
		"pencil": codexMCPEntry{Command: "/path/to/mcp", Args: []string{"--app", "desktop"}},
	}

	if err := mergeMCPIntoTOMLFile(path, "mcp_servers", servers); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readTOML(t, path)
	mcp, ok := got["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers missing or wrong type in %v", got)
	}
	pencil, ok := mcp["pencil"].(map[string]any)
	if !ok {
		t.Fatalf("pencil entry missing")
	}
	if pencil["command"] != "/path/to/mcp" {
		t.Errorf("command = %v", pencil["command"])
	}
}

func TestMergeMCPIntoTOMLFile_PreservesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Pre-existing config with unrelated top-level keys and a stale mcp_servers entry.
	preexisting := []byte(`model = "gpt-5"
approval_policy = "on-request"

[mcp_servers.old]
command = "old-command"
`)
	if err := os.WriteFile(path, preexisting, 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	servers := map[string]any{
		"pencil": codexMCPEntry{Command: "/path/to/mcp"},
	}

	if err := mergeMCPIntoTOMLFile(path, "mcp_servers", servers); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := readTOML(t, path)

	if got["model"] != "gpt-5" {
		t.Errorf("model = %v, want %q (top-level key not preserved)", got["model"], "gpt-5")
	}
	if got["approval_policy"] != "on-request" {
		t.Errorf("approval_policy = %v, want %q", got["approval_policy"], "on-request")
	}

	mcp := got["mcp_servers"].(map[string]any)
	if _, ok := mcp["old"]; ok {
		t.Error("old server should have been replaced")
	}
	if _, ok := mcp["pencil"]; !ok {
		t.Error("pencil server should be present")
	}
}

// TestMergeMCPIntoTOMLFile_DropsComments locks in the documented behavior that
// round-tripping ~/.codex/config.toml loses comments and reformats whitespace.
// Change this test only after the merge implementation switches to a comment-
// preserving TOML library — otherwise users would silently lose annotations.
func TestMergeMCPIntoTOMLFile_DropsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	preexisting := []byte(`# top-level comment
model = "gpt-5"  # inline comment
`)
	if err := os.WriteFile(path, preexisting, 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	servers := map[string]any{
		"pencil": codexMCPEntry{Command: "/path/to/mcp"},
	}

	if err := mergeMCPIntoTOMLFile(path, "mcp_servers", servers); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, _ := os.ReadFile(path)
	for _, marker := range []string{"# top-level comment", "# inline comment"} {
		if strings.Contains(string(out), marker) {
			t.Errorf("expected comment %q to be dropped, but found it in output:\n%s", marker, string(out))
		}
	}
	// The non-comment data should still be there.
	got := readTOML(t, path)
	if got["model"] != "gpt-5" {
		t.Errorf("model = %v, want %q (data should survive even though comments don't)", got["model"], "gpt-5")
	}
}

func TestSyncMCP_WritesCodexFormat(t *testing.T) {
	home := t.TempDir()

	cfg := &config.Config{
		MCP: map[string]config.MCP{
			"pencil": {Command: "/path/to/mcp", Args: []string{"--app", "desktop"}},
		},
	}
	codex := platform.ForNames([]string{"codex"})
	if len(codex) != 1 {
		t.Fatalf("expected one platform match for codex, got %d", len(codex))
	}

	if err := syncMCP(cfg, home, codex, false); err != nil {
		t.Fatalf("syncMCP: %v", err)
	}

	path := filepath.Join(home, ".codex", "config.toml")
	got := readTOML(t, path)
	mcp, ok := got["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers missing or wrong type in %v", got)
	}
	entry, ok := mcp["pencil"].(map[string]any)
	if !ok {
		t.Fatalf("pencil missing from mcp_servers")
	}
	if entry["command"] != "/path/to/mcp" {
		t.Errorf("command = %v", entry["command"])
	}
	if _, ok := entry["cwd"]; ok {
		t.Error("Codex entry should not contain cwd field")
	}
}

func TestSyncMCP_NoMCPs(t *testing.T) {
	home := t.TempDir()
	cfg := &config.Config{}

	// Should be a no-op, no error
	err := syncMCP(cfg, home, platform.All(), false)
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

func readTOML(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var result map[string]any
	if err := toml.Unmarshal(data, &result); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return result
}
