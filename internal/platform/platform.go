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

type AgentTarget struct {
	Dir    string
	Format AgentFormat
}

type MCPTarget struct {
	ConfigPath string
	Key        string
	Format     MCPFormat
}

// Platform describes where a specific AI tool expects its config files.
type Platform struct {
	Key              string // config key, e.g. "antigravity"; multiple targets may share one key
	Name             string
	InstructionsPath string       // relative to home, e.g. ".claude/CLAUDE.md"
	SkillsDir        string       // relative to home, e.g. ".claude/skills". May be shared across platforms (e.g. Codex targets ".agents/skills").
	Agents           *AgentTarget // nil when the platform has no native subagent target
	MCP              *MCPTarget   // nil when the platform has no native MCP configuration
}

// All returns the built-in platform definitions.
func All() []Platform {
	return []Platform{
		{
			Key:              "claude",
			Name:             "Claude",
			InstructionsPath: filepath.Join(".claude", "CLAUDE.md"),
			SkillsDir:        filepath.Join(".claude", "skills"),
			Agents: &AgentTarget{
				Dir:    filepath.Join(".claude", "agents"),
				Format: AgentFormatMarkdown,
			},
			MCP: &MCPTarget{
				ConfigPath: ".claude.json",
				Key:        "mcpServers",
				Format:     MCPFormatStandard,
			},
		},
		{
			Key:              "antigravity",
			Name:             "Antigravity",
			InstructionsPath: filepath.Join(".gemini", "GEMINI.md"),
			SkillsDir:        filepath.Join(".gemini", "config", "skills"),
			Agents: &AgentTarget{
				Dir:    filepath.Join(".gemini", "config", "agents"),
				Format: AgentFormatMarkdown,
			},
			MCP: &MCPTarget{
				ConfigPath: filepath.Join(".gemini", "config", "mcp_config.json"),
				Key:        "mcpServers",
				Format:     MCPFormatStandard,
			},
		},
		{
			Key:              "opencode",
			Name:             "OpenCode",
			InstructionsPath: filepath.Join(".config", "opencode", "AGENTS.md"),
			SkillsDir:        filepath.Join(".config", "opencode", "skills"),
			Agents: &AgentTarget{
				Dir:    filepath.Join(".config", "opencode", "agents"),
				Format: AgentFormatMarkdown,
			},
			MCP: &MCPTarget{
				ConfigPath: filepath.Join(".config", "opencode", "opencode.json"),
				Key:        "mcp",
				Format:     MCPFormatOpenCode,
			},
		},
		{
			Key:              "droid",
			Name:             "Droid",
			InstructionsPath: filepath.Join(".factory", "AGENTS.md"),
			SkillsDir:        filepath.Join(".factory", "skills"),
			Agents: &AgentTarget{
				Dir:    filepath.Join(".factory", "droids"),
				Format: AgentFormatMarkdown,
			},
			MCP: &MCPTarget{
				ConfigPath: filepath.Join(".factory", "mcp.json"),
				Key:        "mcpServers",
				Format:     MCPFormatDroid,
			},
		},
		{
			Key:  "codex",
			Name: "Codex",
			// Codex reads ~/.agents/skills/ — a shared, non-namespaced path.
			InstructionsPath: filepath.Join(".codex", "AGENTS.md"),
			SkillsDir:        filepath.Join(".agents", "skills"),
			Agents: &AgentTarget{
				Dir:    filepath.Join(".codex", "agents"),
				Format: AgentFormatCodexTOML,
			},
			MCP: &MCPTarget{
				ConfigPath: filepath.Join(".codex", "config.toml"),
				Key:        "mcp_servers",
				Format:     MCPFormatCodex,
			},
		},
		{
			Key:              "cursor",
			Name:             "Cursor",
			InstructionsPath: "", // Cursor does not document a user-level AGENTS.md path.
			SkillsDir:        filepath.Join(".cursor", "skills"),
			Agents: &AgentTarget{
				Dir:    filepath.Join(".cursor", "agents"),
				Format: AgentFormatMarkdown,
			},
			MCP: &MCPTarget{
				ConfigPath: filepath.Join(".cursor", "mcp.json"),
				Key:        "mcpServers",
				Format:     MCPFormatCursor,
			},
		},
		{
			Key:              "pi",
			Name:             "Pi",
			InstructionsPath: filepath.Join(".pi", "agent", "AGENTS.md"),
			SkillsDir:        filepath.Join(".pi", "agent", "skills"),
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

// IsSupported reports whether name identifies a built-in platform.
func IsSupported(name string) bool {
	for _, platform := range All() {
		if strings.EqualFold(platform.Key, name) {
			return true
		}
	}
	return false
}
