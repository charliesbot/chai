package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func privateEnv(t *testing.T, home, contents string) string {
	t.Helper()
	path := filepath.Join(home, ".config", "chai", ".env")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveMCPEnvironmentAndPreview(t *testing.T) {
	home := t.TempDir()
	privateEnv(t, home, "# private values\nCHAI_FILE_ONLY='file$literal'\nCHAI_PROCESS=ignored\nCHAI_NO_EXEC='$(touch should-not-exist)'\n")
	t.Setenv("CHAI_PROCESS", "process-value")
	input := map[string]MCP{"local": {
		Command: "${CHAI_PROCESS}", Args: []string{"prefix-${CHAI_FILE_ONLY}", "$UNCHANGED", "$${ESCAPED}"},
		Env: map[string]string{"TOKEN": "${CHAI_FILE_ONLY}", "LITERAL": "private-literal", "COMMAND": "${CHAI_NO_EXEC}"},
	}}
	result, err := ResolveMCP(input, home)
	if err != nil {
		t.Fatal(err)
	}
	got := result.Values["local"]
	if got.Command != "process-value" || got.Args[0] != "prefix-file$literal" || got.Args[1] != "$UNCHANGED" || got.Args[2] != "${ESCAPED}" {
		t.Fatalf("incorrect substitution: %+v", got)
	}
	if got.Env["COMMAND"] != "$(touch should-not-exist)" {
		t.Fatal("shell expression was not preserved as data")
	}
	preview := result.Preview["local"]
	if preview.Command != "<redacted>" || preview.Args[0] != "<redacted>" || preview.Env["LITERAL"] != "<redacted>" {
		t.Fatal("preview leaked values")
	}
	if input["local"].Command != "${CHAI_PROCESS}" || input["local"].Args[0] != "prefix-${CHAI_FILE_ONLY}" {
		t.Fatal("manifest mutated")
	}
	got.Env["TOKEN"] = "changed"
	got.Args[0] = "changed"
	if input["local"].Env["TOKEN"] != "${CHAI_FILE_ONLY}" {
		t.Fatal("map aliases source")
	}
	if _, ok := os.LookupEnv("CHAI_FILE_ONLY"); ok {
		t.Fatal("dotenv changed process environment")
	}
}

func TestResolveMCPRejectsMissingAndMalformedReferences(t *testing.T) {
	t.Setenv("CHAI_EMPTY", "")
	for _, raw := range []string{"${CHAI_EMPTY}", "${CHAI_TEST_NOT_SET_579}", "${}", "${BAD-NAME}", "${CHAI_EMPTY:-fallback}", "${UNCLOSED"} {
		_, err := ResolveMCP(map[string]MCP{"test": {Command: "test", Env: map[string]string{"TOKEN": raw}}}, t.TempDir())
		if err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestResolveMCPEnvFileErrorsDoNotDiscloseContents(t *testing.T) {
	home := t.TempDir()
	path := privateEnv(t, home, "TOKEN='secret-sentinel\n")
	servers := map[string]MCP{"test": {Command: "test"}}
	if _, err := ResolveMCP(servers, home); err == nil || strings.Contains(err.Error(), "secret-sentinel") {
		t.Fatalf("unsafe parse error: %v", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveMCP(servers, home); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("expected permissions error: %v", err)
	}
}

func TestValidateMCPTransports(t *testing.T) {
	for _, server := range []MCP{
		{}, {Command: "local", URL: "https://example.com/mcp"}, {URL: "file:///secret"},
		{URL: "https://user:password@example.com"}, {URL: "https://example.com/#fragment"},
		{URL: "https://example.com", Args: []string{"arg"}}, {URL: "https://example.com", Env: map[string]string{"A": "B"}},
		{URL: "https://example.com", CWD: "/tmp"}, {Command: "local", Headers: map[string]string{"X-Key": "value"}},
		{URL: "https://example.com", Headers: map[string]string{"Bad Header": "value"}},
		{URL: "https://example.com", Headers: map[string]string{"X-Key": "bad\nheader"}},
	} {
		if err := ValidateMCP(map[string]MCP{"test": server}); err == nil {
			t.Errorf("accepted invalid transport: %+v", server)
		}
	}
	for _, server := range []MCP{{Command: "npx"}, {URL: "http://localhost:8000/mcp"}, {URL: "https://stitch.googleapis.com/mcp", Headers: map[string]string{"X-Goog-Api-Key": "${TOKEN}"}}} {
		if err := ValidateMCP(map[string]MCP{"test": server}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolveMCPValidatesExpandedValues(t *testing.T) {
	t.Setenv("CHAI_BAD_URL", "not-a-url-secret-sentinel")
	t.Setenv("CHAI_BAD_HEADER", "secret-sentinel\r\nInjected: yes")
	for _, server := range []MCP{{URL: "${CHAI_BAD_URL}"}, {URL: "https://example.com", Headers: map[string]string{"X-Key": "${CHAI_BAD_HEADER}"}}} {
		if _, err := ResolveMCP(map[string]MCP{"test": server}, t.TempDir()); err == nil || strings.Contains(err.Error(), "secret-sentinel") {
			t.Fatalf("unsafe validation: %v", err)
		}
	}
}

func TestRemoteMCPManifestRoundTripPreservesReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chai.toml")
	cfg := &Config{Platforms: []string{"codex"}, MCP: map[string]MCP{"stitch": {URL: "https://stitch.googleapis.com/mcp", Headers: map[string]string{"X-Goog-Api-Key": "${TOKEN}"}}}}
	if err := SaveAtomic(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.MCP, loaded.MCP) {
		t.Fatal("remote config changed on round trip")
	}
}

func TestConfigParseErrorsDoNotEchoMCPSecrets(t *testing.T) {
	for _, content := range []string{
		"platforms = [\"cursor\"]\n[mcp.test]\ncommand = \"test\"\nunknown = \"secret-sentinel\"\n",
		"platforms = [\"cursor\"]\n[mcp.test]\ncommand = \"test\"\nargs = \"secret-sentinel\"\n",
	} {
		_, err := load("chai.toml", []byte(content))
		if err == nil || strings.Contains(err.Error(), "secret-sentinel") {
			t.Fatalf("unsafe parse error: %v", err)
		}
	}
}
