package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

	claudeOk := true
	geminiOk := true
	platforms := platform.All()
	for _, p := range platforms {
		dest := filepath.Join(home, p.MCPConfigPath)
		if err := mergeMCPIntoFile(dest, p.MCPKey, servers); err != nil {
			if p.Name == "Claude" {
				claudeOk = false
			} else {
				geminiOk = false
			}
		}
	}

	// Collect server names
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println(ui.Box("mcpServers", len(servers), ui.BoolState(claudeOk), ui.BoolState(geminiOk), names))

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
		// Resolve @name in args (only for args starting with @)
		for i, arg := range entry.Args {
			if strings.HasPrefix(arg, "@") {
				resolved, err := resolve.PathWithHome(arg, home)
				if err != nil {
					continue // not a dep reference, leave as-is (e.g. @angular/cli is an npm scope)
				}
				// Only apply if the resolved path actually exists
				if _, statErr := os.Stat(resolved); statErr == nil {
					entry.Args[i] = resolved
				}
			}
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
