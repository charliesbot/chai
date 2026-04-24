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

// mcpEntry is the JSON structure written per MCP server for standard-format platforms
// (Claude, Gemini, Antigravity).
type mcpEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
	CWD     string            `json:"cwd,omitempty"`
}

// opencodeMCPEntry is the JSON structure OpenCode expects in opencode.json under "mcp".
// Command is a single array that bundles what we call Command+Args.
type opencodeMCPEntry struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command"`
	Environment map[string]string `json:"environment,omitempty"`
	Enabled     bool              `json:"enabled"`
}

// syncMCP writes MCP server definitions to each platform's config file,
// using each platform's preferred entry shape.
func syncMCP(cfg *config.Config, home string, platforms []platform.Platform, dryRun bool) error {
	if len(cfg.MCP) == 0 {
		return nil
	}

	standard, err := buildMCPServers(cfg.MCP, home)
	if err != nil {
		return err
	}
	opencode := buildOpenCodeMCPServers(standard)

	// OpenCode has no cwd equivalent — flag servers that define one so users
	// don't silently get a different working directory across platforms.
	hasOpenCode := false
	for _, p := range platforms {
		if p.MCPFormat == platform.MCPFormatOpenCode {
			hasOpenCode = true
			break
		}
	}
	if hasOpenCode && !dryRun {
		var dropped []string
		for name, e := range standard {
			if e.CWD != "" {
				dropped = append(dropped, name)
			}
		}
		if len(dropped) > 0 {
			sort.Strings(dropped)
			fmt.Printf("  %s %s %s\n", ui.Warning.Render("!"), ui.Muted.Render("cwd not supported by OpenCode, ignored for:"), ui.Bold.Render(strings.Join(dropped, ", ")))
		}
	}

	entriesFor := func(p platform.Platform) map[string]any {
		out := make(map[string]any, len(standard))
		switch p.MCPFormat {
		case platform.MCPFormatOpenCode:
			for name, e := range opencode {
				out[name] = e
			}
		default:
			for name, e := range standard {
				out[name] = e
			}
		}
		return out
	}

	if dryRun {
		fmt.Println(ui.Label.Render("mcp servers"))

		// Group platforms by format so users see which platforms get which shape.
		formatOrder := make([]platform.MCPFormat, 0)
		byFormat := make(map[platform.MCPFormat][]platform.Platform)
		for _, p := range platforms {
			if _, ok := byFormat[p.MCPFormat]; !ok {
				formatOrder = append(formatOrder, p.MCPFormat)
			}
			byFormat[p.MCPFormat] = append(byFormat[p.MCPFormat], p)
		}

		for _, f := range formatOrder {
			group := byFormat[f]
			fmt.Printf("  %s\n", ui.Muted.Render(platformNames(group)+":"))
			preview, err := json.MarshalIndent(map[string]any{group[0].MCPKey: entriesFor(group[0])}, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling MCP preview: %w", err)
			}
			fmt.Println(ui.JSONBlock.Render(string(preview)))
		}
		fmt.Println()

		fmt.Println(ui.Label.Render("  targets"))
		for _, p := range platforms {
			dest := filepath.Join(home, p.MCPConfigPath)
			fmt.Printf("  %s %s %s\n", ui.Arrow(), ui.Bold.Render(p.Name), ui.Muted.Render(dest))
		}
		fmt.Println()
		return nil
	}

	status := newPlatformStatus(platforms)

	// Dedup by destination path + MCP key + format so platforms that share a
	// config file only get one merge pass.
	type mcpTarget struct {
		path   string
		key    string
		format platform.MCPFormat
	}
	seen := make(map[mcpTarget][]platform.Platform)
	order := make([]mcpTarget, 0, len(platforms))
	for _, p := range platforms {
		t := mcpTarget{path: filepath.Join(home, p.MCPConfigPath), key: p.MCPKey, format: p.MCPFormat}
		if _, ok := seen[t]; !ok {
			order = append(order, t)
		}
		seen[t] = append(seen[t], p)
	}

	for _, t := range order {
		entries := entriesFor(seen[t][0])
		if err := mergeMCPIntoFile(t.path, t.key, entries); err != nil {
			names := make([]string, len(seen[t]))
			for i, p := range seen[t] {
				status.setFailed(p.Name)
				names[i] = p.Name
			}
			fmt.Printf("  %s %s %s\n", ui.Warning.Render("!"), ui.Bold.Render(strings.Join(names, " + ")), ui.Muted.Render(err.Error()))
		}
	}

	// Collect server names
	names := make([]string, 0, len(standard))
	for name := range standard {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println(ui.Box("mcp servers", len(standard), status.statuses(), names))

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

// buildOpenCodeMCPServers translates standard entries into OpenCode's shape.
// OpenCode has no `cwd` field; if a server sets one, it is dropped silently —
// callers running mixed platforms should rely on shell setup instead.
func buildOpenCodeMCPServers(standard map[string]mcpEntry) map[string]opencodeMCPEntry {
	out := make(map[string]opencodeMCPEntry, len(standard))
	for name, e := range standard {
		cmd := make([]string, 0, 1+len(e.Args))
		cmd = append(cmd, e.Command)
		cmd = append(cmd, e.Args...)
		out[name] = opencodeMCPEntry{
			Type:        "local",
			Command:     cmd,
			Environment: e.Env,
			Enabled:     true,
		}
	}
	return out
}

// mergeMCPIntoFile reads an existing JSON file, replaces the mcpKey, and writes it back atomically.
func mergeMCPIntoFile(path, mcpKey string, servers map[string]any) error {
	existing := make(map[string]any)

	data, err := os.ReadFile(path)
	if err == nil {
		// Some platforms (e.g. Antigravity) create this file empty on first launch;
		// treat zero-byte files as a fresh start rather than a parse error.
		if len(data) > 0 {
			if err := json.Unmarshal(data, &existing); err != nil {
				return fmt.Errorf("parsing existing config %s: %w", path, err)
			}
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
