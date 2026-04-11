package platform

import "path/filepath"

// Platform describes where a specific AI tool expects its config files.
type Platform struct {
	Name             string
	InstructionsPath string // relative to home, e.g. ".claude/CLAUDE.md"
	SkillsDir        string // relative to home, e.g. ".claude/skills"
	AgentsDir        string // relative to home, e.g. ".claude/subagents"
	MCPConfigPath    string // relative to home, e.g. ".claude.json"
	MCPKey           string // JSON key for MCP servers, e.g. "mcpServers"
}

// All returns the built-in platform definitions.
func All() []Platform {
	return []Platform{
		{
			Name:             "Claude",
			InstructionsPath: filepath.Join(".claude", "CLAUDE.md"),
			SkillsDir:        filepath.Join(".claude", "skills"),
			AgentsDir:        filepath.Join(".claude", "subagents"),
			MCPConfigPath:    ".claude.json",
			MCPKey:           "mcpServers",
		},
		{
			Name:             "Gemini",
			InstructionsPath: filepath.Join(".gemini", "GEMINI.md"),
			SkillsDir:        filepath.Join(".gemini", "skills"),
			AgentsDir:        filepath.Join(".gemini", "subagents"),
			MCPConfigPath:    filepath.Join(".gemini", "settings.json"),
			MCPKey:           "mcpServers",
		},
	}
}
