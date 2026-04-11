package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/resolve"
	"github.com/charliesbot/chai/internal/ui"
)

// mcpEntry is the JSON structure written per MCP server.
type mcpEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
	CWD     string            `json:"cwd,omitempty"`
}

// syncMCP writes mcpServers to each platform's config file.
func syncMCP(cfg *config.Config, home string, dryRun bool) error {
	if len(cfg.MCP) == 0 {
		return nil
	}

	servers, err := buildMCPServers(cfg.MCP, home)
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Println(ui.Label.Render("mcpServers"))

		mcpJSON, err := json.MarshalIndent(map[string]any{"mcpServers": servers}, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling MCP preview: %w", err)
		}
		fmt.Println(ui.JSONBlock.Render(string(mcpJSON)))
		fmt.Println()

		fmt.Println(ui.Label.Render("  targets"))
		platforms := platform.All()
		for _, p := range platforms {
			dest := filepath.Join(home, p.MCPConfigPath)
			fmt.Printf("  %s %s %s\n", ui.Arrow(), ui.Bold.Render(p.Name), ui.Muted.Render(dest))
		}
		fmt.Println()
		return nil
	}

	platforms := platform.All()
	for _, p := range platforms {
		dest := filepath.Join(home, p.MCPConfigPath)
		if err := mergeMCPIntoFile(dest, p.MCPKey, servers); err != nil {
			return fmt.Errorf("writing MCP config for %s to %s: %w", p.Name, dest, err)
		}
		fmt.Println(ui.SyncedLine(p.Name, dest))
	}

	return nil
}

// buildMCPServers resolves @name in cwd fields and builds the servers map.
func buildMCPServers(mcps map[string]config.MCP, home string) (map[string]mcpEntry, error) {
	servers := make(map[string]mcpEntry, len(mcps))
	for name, m := range mcps {
		entry := mcpEntry{
			Command: m.Command,
			Args:    m.Args,
			Env:     m.Env,
		}
		if m.CWD != "" {
			resolved, err := resolve.PathWithHome(m.CWD, home)
			if err != nil {
				return nil, fmt.Errorf("resolving cwd for mcp %q: %w", name, err)
			}
			entry.CWD = resolved
		}
		servers[name] = entry
	}
	return servers, nil
}

// mergeMCPIntoFile reads an existing JSON file, replaces the mcpKey, and writes it back atomically.
func mergeMCPIntoFile(path, mcpKey string, servers map[string]mcpEntry) error {
	existing := make(map[string]any)

	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parsing existing config %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	existing[mcpKey] = servers

	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	out = append(out, '\n')

	return atomicWrite(path, out)
}
