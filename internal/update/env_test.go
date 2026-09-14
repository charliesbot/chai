package update

import (
	"context"
	"fmt"
	"testing"

	"github.com/charliesbot/chai/internal/config"
)

func TestMissingMCPSecretPreventsRefresh(t *testing.T) {
	t.Setenv("CHAI_PREFLIGHT_SECRET", "")
	cfg := &config.Config{Platforms: []string{"cursor"}, MCP: map[string]config.MCP{"stitch": {URL: "https://example.com", Headers: map[string]string{"X-Key": "${CHAI_PREFLIGHT_SECRET}"}}}, Skills: config.Skills{GitHub: []config.GitHubSkills{{URL: "https://github.com/example/skills", Include: []string{"test"}}}}}
	checked := false
	err := RunWithHome(context.Background(), cfg, t.TempDir(), Options{CheckGit: func(context.Context) error { checked = true; return fmt.Errorf("unexpected Git check") }})
	if err == nil || checked {
		t.Fatalf("refresh preflight reached before secret validation: checked=%v err=%v", checked, err)
	}
}
