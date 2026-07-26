package platform

import (
	"path/filepath"
	"strings"
)

// MCPFormat identifies the on-disk shape a platform expects for an MCP entry.
type MCPFormat string

const (
	// MCPFormatStandard is the Claude / Antigravity shape:
	//   {"command": "npx", "args": [...], "env": {...}, "cwd": "..."}
	MCPFormatStandard MCPFormat = "standard"

	// MCPFormatOpenCode is the OpenCode shape:
	//   {"type": "local", "command": ["npx", ...], "environment": {...}, "enabled": true}
	MCPFormatOpenCode MCPFormat = "opencode"

	// MCPFormatCodex is the Codex shape — same fields as Standard minus cwd,
	// but the host file is TOML rather than JSON.
	MCPFormatCodex MCPFormat = "codex"

	// MCPFormatDroid is the Droid shape in ~/.factory/mcp.json:
	//   {"type": "stdio", "command": "npx", "args": [...], "env": {...}, "disabled": false}
	MCPFormatDroid MCPFormat = "droid"

	// MCPFormatCursor is the Cursor shape in ~/.cursor/mcp.json:
	//   {"type": "stdio", "command": "npx", "args": [...], "env": {...}}
	MCPFormatCursor MCPFormat = "cursor"
)

// AgentFormat identifies how a platform expects subagent definitions on disk.
type AgentFormat string

const (
	AgentFormatMarkdown  AgentFormat = "markdown"
	AgentFormatCodexTOML AgentFormat = "codex-toml"
)

// Platform describes where a specific AI tool expects its config files.
type Platform struct {
	Key              string // config key, e.g. "antigravity"; multiple targets may share one key
	Name             string
	InstructionsPath string // relative to home, e.g. ".claude/CLAUDE.md"
	SkillsDir        string // relative to home, e.g. ".claude/skills". May be shared across platforms (e.g. Codex targets ".agents/skills").
	AgentsDir        string // relative to home, e.g. ".claude/subagents"; "" = platform has no subagent target
	AgentFormat      AgentFormat
	MCPConfigPath    string    // relative to home, e.g. ".claude.json"
	MCPKey           string    // JSON key for MCP servers, e.g. "mcpServers"
	MCPFormat        MCPFormat // on-disk shape of each MCP entry
}

// All returns the built-in platform definitions.
func All() []Platform {
	return []Platform{
		{
			Key:              "claude",
			Name:             "Claude",
			InstructionsPath: filepath.Join(".claude", "CLAUDE.md"),
			SkillsDir:        filepath.Join(".claude", "skills"),
			AgentsDir:        filepath.Join(".claude", "agents"),
			AgentFormat:      AgentFormatMarkdown,
			MCPConfigPath:    ".claude.json",
			MCPKey:           "mcpServers",
			MCPFormat:        MCPFormatStandard,
		},
		{
			Key:              "antigravity",
			Name:             "Antigravity IDE",
			InstructionsPath: filepath.Join(".gemini", "GEMINI.md"),
			SkillsDir:        filepath.Join(".gemini", "antigravity-ide", "skills"),
			AgentsDir:        "", // Antigravity does not expose a user subagents directory
			AgentFormat:      AgentFormatMarkdown,
			MCPConfigPath:    filepath.Join(".gemini", "config", "mcp_config.json"),
			MCPKey:           "mcpServers",
			MCPFormat:        MCPFormatStandard,
		},
		{
			Key:              "antigravity",
			Name:             "Antigravity",
			InstructionsPath: filepath.Join(".gemini", "GEMINI.md"),
			SkillsDir:        filepath.Join(".gemini", "antigravity", "skills"),
			AgentsDir:        "", // Antigravity does not expose a user subagents directory
			AgentFormat:      AgentFormatMarkdown,
			MCPConfigPath:    filepath.Join(".gemini", "config", "mcp_config.json"),
			MCPKey:           "mcpServers",
			MCPFormat:        MCPFormatStandard,
		},
		{
			Key:              "antigravity",
			Name:             "Antigravity CLI",
			InstructionsPath: filepath.Join(".gemini", "GEMINI.md"),
			SkillsDir:        filepath.Join(".gemini", "antigravity-cli", "skills"),
			AgentsDir:        "",
			AgentFormat:      AgentFormatMarkdown,
			MCPConfigPath:    filepath.Join(".gemini", "config", "mcp_config.json"),
			MCPKey:           "mcpServers",
			MCPFormat:        MCPFormatStandard,
		},
		{
			Key:              "opencode",
			Name:             "OpenCode",
			InstructionsPath: filepath.Join(".config", "opencode", "AGENTS.md"),
			SkillsDir:        filepath.Join(".config", "opencode", "skills"),
			AgentsDir:        filepath.Join(".config", "opencode", "agents"),
			AgentFormat:      AgentFormatMarkdown,
			MCPConfigPath:    filepath.Join(".config", "opencode", "opencode.json"),
			MCPKey:           "mcp",
			MCPFormat:        MCPFormatOpenCode,
		},
		{
			Key:              "droid",
			Name:             "Droid",
			InstructionsPath: filepath.Join(".factory", "AGENTS.md"),
			SkillsDir:        filepath.Join(".factory", "skills"),
			AgentsDir:        filepath.Join(".factory", "droids"),
			AgentFormat:      AgentFormatMarkdown,
			MCPConfigPath:    filepath.Join(".factory", "mcp.json"),
			MCPKey:           "mcpServers",
			MCPFormat:        MCPFormatDroid,
		},
		{
			Key:  "codex",
			Name: "Codex",
			// Codex reads ~/.agents/skills/ — a shared, non-namespaced path.
			InstructionsPath: filepath.Join(".codex", "AGENTS.md"),
			SkillsDir:        filepath.Join(".agents", "skills"),
			AgentsDir:        filepath.Join(".codex", "agents"),
			AgentFormat:      AgentFormatCodexTOML,
			MCPConfigPath:    filepath.Join(".codex", "config.toml"),
			MCPKey:           "mcp_servers",
			MCPFormat:        MCPFormatCodex,
		},
		{
			Key:              "cursor",
			Name:             "Cursor",
			InstructionsPath: "", // Cursor does not document a user-level AGENTS.md path.
			SkillsDir:        filepath.Join(".cursor", "skills"),
			AgentsDir:        filepath.Join(".cursor", "agents"),
			AgentFormat:      AgentFormatMarkdown,
			MCPConfigPath:    filepath.Join(".cursor", "mcp.json"),
			MCPKey:           "mcpServers",
			MCPFormat:        MCPFormatCursor,
		},
	}
}

// ForNames returns only the platforms whose names match the given list (case-insensitive).
func ForNames(names []string) []Platform {
	allowed := make(map[string]bool, len(names))
	for _, n := range names {
		allowed[strings.ToLower(n)] = true
	}

	all := All()
	filtered := make([]Platform, 0, len(names))
	for _, p := range all {
		if allowed[strings.ToLower(p.Key)] {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// HasPlatform reports whether the given platform name is in the list (case-insensitive).
func HasPlatform(names []string, name string) bool {
	target := strings.ToLower(name)
	for _, n := range names {
		if strings.ToLower(n) == target {
			return true
		}
	}
	return false
}
