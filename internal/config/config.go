package config

import (
	"fmt"
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

type Config struct {
	Instructions string         `toml:"instructions"`
	Deps         map[string]Dep `toml:"-"`
	Skills       Skills         `toml:"skills"`
	Subagents    Subagents      `toml:"subagents"`
	MCP          map[string]MCP `toml:"mcp"`
	Gemini       GeminiConfig   `toml:"gemini"`
}

type GeminiConfig struct {
	Extensions map[string]string `toml:"extensions"`
}

// Dep represents a dependency — either a simple URL string or a table with url + build.
type Dep struct {
	URL   string
	Build string
}

type Skills struct {
	Paths []string `toml:"paths"`
}

type Subagents struct {
	Paths []string `toml:"paths"`
}

type MCP struct {
	Command string            `toml:"command"`
	Args    []string          `toml:"args"`
	Env     map[string]string `toml:"env"`
	CWD     string            `toml:"cwd"`
}

// rawConfig is the intermediate TOML representation.
type rawConfig struct {
	Instructions string         `toml:"instructions"`
	Deps         map[string]any `toml:"deps"`
	Skills       Skills         `toml:"skills"`
	Subagents    Subagents      `toml:"subagents"`
	MCP          map[string]MCP `toml:"mcp"`
	Gemini       GeminiConfig   `toml:"gemini"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s (run 'chai init' to create one)", path)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var raw rawConfig
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	deps, err := parseDeps(raw.Deps)
	if err != nil {
		return nil, fmt.Errorf("parsing deps in %s: %w", path, err)
	}

	cfg := &Config{
		Instructions: raw.Instructions,
		Deps:         deps,
		Skills:       raw.Skills,
		Subagents:    raw.Subagents,
		MCP:          raw.MCP,
		Gemini:       raw.Gemini,
	}

	return cfg, nil
}

func parseDeps(raw map[string]any) (map[string]Dep, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	deps := make(map[string]Dep, len(raw))
	for name, v := range raw {
		switch val := v.(type) {
		case string:
			deps[name] = Dep{URL: val}
		case map[string]any:
			d := Dep{}
			if url, ok := val["url"].(string); ok {
				d.URL = url
			} else {
				return nil, fmt.Errorf("dep %q: table requires a 'url' field", name)
			}
			if build, ok := val["build"].(string); ok {
				d.Build = build
			}
			deps[name] = d
		default:
			return nil, fmt.Errorf("dep %q: must be a string or table, got %T", name, v)
		}
	}

	return deps, nil
}
