package sync

import (
	"context"
	"encoding/json"
	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/platform"
	toml "github.com/pelletier/go-toml/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func remoteConfig(t *testing.T, home string) *config.Config {
	t.Helper()
	path := filepath.Join(home, "chai.toml")
	content := `platforms = ["claude", "antigravity", "codex", "opencode", "droid", "cursor"]
instructions = ["~/instructions.md"]
[mcp.stitch]
url = "https://stitch.googleapis.com/mcp"
[mcp.stitch.headers]
X-Goog-Api-Key = "${CHAI_TEST_STITCH_KEY}"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "instructions.md"), []byte("Instructions"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeEnvFile(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "chai")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteMCPTranslationsAndPrivateWrites(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CHAI_TEST_STITCH_KEY", "process-secret")
	writeEnvFile(t, home, "CHAI_TEST_STITCH_KEY=file-secret\n")
	cfg := remoteConfig(t, home)
	for _, p := range platform.ForNames(cfg.Platforms) {
		if p.MCP == nil {
			continue
		}
		path := filepath.Join(home, p.MCP.ConfigPath)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		content := []byte("{\"unrelated\":true}")
		if p.Key == "codex" {
			content = []byte("unrelated = true\n")
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(filepath.Join(home, "chai.toml"))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := captureStdout(t, func() error { return RunWithHome(context.Background(), cfg, home, Options{DryRun: true}) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(preview, "process-secret") || strings.Contains(preview, "file-secret") {
		t.Fatal("dry run exposed a secret")
	}
	if err := RunWithHome(context.Background(), cfg, home, Options{}); err != nil {
		t.Fatal(err)
	}
	for _, p := range platform.ForNames(cfg.Platforms) {
		if p.MCP == nil {
			continue
		}
		path := filepath.Join(home, p.MCP.ConfigPath)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var root map[string]any
		if p.Key == "codex" {
			err = toml.Unmarshal(data, &root)
		} else {
			err = json.Unmarshal(data, &root)
		}
		if err != nil {
			t.Fatal(err)
		}
		if root["unrelated"] != true {
			t.Fatalf("%s lost unrelated config", p.Name)
		}
		entry := root[p.MCP.Key].(map[string]any)["stitch"].(map[string]any)
		urlKey, headerKey := "url", "headers"
		if p.Key == "antigravity" {
			urlKey = "serverUrl"
		}
		if p.Key == "codex" {
			headerKey = "http_headers"
		}
		if entry[urlKey] != "https://stitch.googleapis.com/mcp" {
			t.Fatalf("%s endpoint missing", p.Name)
		}
		if entry[headerKey].(map[string]any)["X-Goog-Api-Key"] != "process-secret" {
			t.Fatalf("%s header missing", p.Name)
		}
		for _, field := range []string{"command", "args", "env", "environment", "cwd"} {
			if _, ok := entry[field]; ok {
				t.Fatalf("%s has local-only field %s", p.Name, field)
			}
		}
		if p.Key == "claude" || p.Key == "droid" {
			if entry["type"] != "http" {
				t.Fatalf("%s type = %v", p.Name, entry["type"])
			}
		}
		if p.Key == "opencode" && entry["type"] != "remote" {
			t.Fatal("OpenCode must use remote type")
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("%s permissions not private: %v", p.Name, err)
		}
	}
	after, _ := os.ReadFile(filepath.Join(home, "chai.toml"))
	if string(before) != string(after) {
		t.Fatal("sync modified manifest")
	}
}

func TestRemoteMCPMissingSecretBeforeWrites(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CHAI_TEST_STITCH_KEY", "")
	writeEnvFile(t, home, "CHAI_TEST_STITCH_KEY=fallback-must-not-win\n")
	cfg := remoteConfig(t, home)
	for _, dryRun := range []bool{false, true} {
		err := RunWithHome(context.Background(), cfg, home, Options{DryRun: dryRun})
		if err == nil || !strings.Contains(err.Error(), "CHAI_TEST_STITCH_KEY") {
			t.Fatalf("want missing variable error, got %v", err)
		}
	}
	for _, dir := range []string{".claude", ".gemini", ".codex", ".chai"} {
		if _, err := os.Stat(filepath.Join(home, dir)); !os.IsNotExist(err) {
			t.Fatalf("wrote %s before validation", dir)
		}
	}
}
