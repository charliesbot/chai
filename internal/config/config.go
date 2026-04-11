package config

import (
	"fmt"
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

type Config struct {
	Instructions string            `toml:"instructions"`
	Deps         map[string]string `toml:"deps"`
	Skills       Skills            `toml:"skills"`
	Subagents    Subagents         `toml:"subagents"`
	MCP          map[string]MCP    `toml:"mcp"`
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

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s (run 'chai init' to create one)", path)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &cfg, nil
}
