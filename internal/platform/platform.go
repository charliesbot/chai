package platform

import (
	"path/filepath"
	"strings"
)

// MCPFormat identifies the on-disk shape a platform expects for an MCP entry.
type MCPFormat string

const (
	// MCPFormatStandard is the Claude / Gemini / Antigravity shape:
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
)

// Platform describes where a specific AI tool expects its config files.
type Platform struct {
	Name             string
	InstructionsPath string    // relative to home, e.g. ".claude/CLAUDE.md"
	SkillsDir        string    // relative to home, e.g. ".claude/skills". May be shared across platforms (e.g. Gemini and Codex both target ".agents/skills").
	AgentsDir        string    // relative to home, e.g. ".claude/subagents"; "" = platform has no markdown subagent target
	MCPConfigPath    string    // relative to home, e.g. ".claude.json"
	MCPKey           string    // JSON key for MCP servers, e.g. "mcpServers"
	MCPFormat        MCPFormat // on-disk shape of each MCP entry
}

// All returns the built-in platform definitions.
func All() []Platform {
	return []Platform{
		{
			Name:             "Claude",
			InstructionsPath: filepath.Join(".claude", "CLAUDE.md"),
			SkillsDir:        filepath.Join(".claude", "skills"),
			AgentsDir:        filepath.Join(".claude", "agents"),
			MCPConfigPath:    ".claude.json",
			MCPKey:           "mcpServers",
			MCPFormat:        MCPFormatStandard,
		},
		{
			Name:             "Gemini",
			InstructionsPath: filepath.Join(".gemini", "GEMINI.md"),
			// Gemini auto-discovers skills from ~/.agents/skills/ (shared with Codex)
			// in addition to ~/.gemini/skills/, with .agents/ taking precedence on
			// conflict. Writing both paths produced "skill conflict" warnings on
			// launch, so chai writes only the shared path. Works whether Codex is
			// enabled or not.
			SkillsDir:     filepath.Join(".agents", "skills"),
			AgentsDir:     filepath.Join(".gemini", "agents"),
			MCPConfigPath: filepath.Join(".gemini", "settings.json"),
			MCPKey:        "mcpServers",
			MCPFormat:     MCPFormatStandard,
		},
		{
			Name:             "Antigravity",
			InstructionsPath: filepath.Join(".gemini", "GEMINI.md"),
			SkillsDir:        filepath.Join(".gemini", "antigravity", "skills"),
			AgentsDir:        "", // Antigravity does not expose a user subagents directory
			MCPConfigPath:    filepath.Join(".gemini", "antigravity", "mcp_config.json"),
			MCPKey:           "mcpServers",
			MCPFormat:        MCPFormatStandard,
		},
		{
			Name:             "OpenCode",
			InstructionsPath: filepath.Join(".config", "opencode", "AGENTS.md"),
			SkillsDir:        filepath.Join(".config", "opencode", "skills"),
			AgentsDir:        filepath.Join(".config", "opencode", "agents"),
			MCPConfigPath:    filepath.Join(".config", "opencode", "opencode.json"),
			MCPKey:           "mcp",
			MCPFormat:        MCPFormatOpenCode,
		},
		{
			Name:             "Droid",
			InstructionsPath: filepath.Join(".factory", "AGENTS.md"),
			SkillsDir:        filepath.Join(".factory", "skills"),
			AgentsDir:        filepath.Join(".factory", "droids"),
			MCPConfigPath:    filepath.Join(".factory", "mcp.json"),
			MCPKey:           "mcpServers",
			MCPFormat:        MCPFormatDroid,
		},
		{
			Name: "Codex",
			// Codex reads ~/.agents/skills/ — a shared, non-namespaced path. If
			// another tool starts writing there, dirty detection may flag files
			// chai didn't write.
			InstructionsPath: filepath.Join(".codex", "AGENTS.md"),
			SkillsDir:        filepath.Join(".agents", "skills"),
			AgentsDir:        "", // Codex agents are TOML files; chai does not translate from markdown.
			MCPConfigPath:    filepath.Join(".codex", "config.toml"),
			MCPKey:           "mcp_servers",
			MCPFormat:        MCPFormatCodex,
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
		if allowed[strings.ToLower(p.Name)] {
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
