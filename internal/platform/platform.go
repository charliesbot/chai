package platform

import (
	"path/filepath"
	"strings"
)

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
			AgentsDir:        filepath.Join(".claude", "agents"),
			MCPConfigPath:    ".claude.json",
			MCPKey:           "mcpServers",
		},
		{
			Name:             "Gemini",
			InstructionsPath: filepath.Join(".gemini", "GEMINI.md"),
			SkillsDir:        filepath.Join(".gemini", "skills"),
			AgentsDir:        filepath.Join(".gemini", "agents"),
			MCPConfigPath:    filepath.Join(".gemini", "settings.json"),
			MCPKey:           "mcpServers",
		},
		{
			Name:             "Antigravity",
			InstructionsPath: filepath.Join(".gemini", "GEMINI.md"),
			SkillsDir:        filepath.Join(".gemini", "antigravity", "skills"),
			AgentsDir:        "", // Antigravity does not expose a user subagents directory
			MCPConfigPath:    filepath.Join(".gemini", "antigravity", "mcp_config.json"),
			MCPKey:           "mcpServers",
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
