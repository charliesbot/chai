package add

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charliesbot/chai/internal/config"
)

func TestMissingMCPSecretPreventsManifestWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CHAI_PREFLIGHT_SECRET", "")
	local := filepath.Join(home, "local")
	writeSkill(t, local, "test")
	manifest := filepath.Join(home, "chai.toml")
	cfg := &config.Config{Platforms: []string{"cursor"}, MCP: map[string]config.MCP{"stitch": {URL: "https://example.com", Headers: map[string]string{"X-Key": "${CHAI_PREFLIGHT_SECRET}"}}}}
	if err := RunWithHome(context.Background(), cfg, manifest, home, []string{local}, Options{}); err == nil {
		t.Fatal("accepted missing secret")
	}
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatal("manifest written before secret validation")
	}
}
