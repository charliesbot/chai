package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charliesbot/chai/internal/githubskill"
	platformpkg "github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/skill"
	toml "github.com/pelletier/go-toml/v2"
)

type Config struct {
	Platforms    []string          `toml:"platforms"`
	Instructions []string          `toml:"instructions"`
	Deps         map[string]Dep    `toml:"-"`
	Skills       Skills            `toml:"skills"`
	Subagents    Subagents         `toml:"subagents"`
	MCP          map[string]MCP    `toml:"mcp"`
	Antigravity  AntigravityConfig `toml:"antigravity"`
	Droid        DroidConfig       `toml:"droid"`
}

type AntigravityConfig struct {
	Plugins map[string]string `toml:"plugins"`
}

type DroidConfig struct {
	CustomModels []CustomModel `toml:"custom_models"`
}

type CustomModel struct {
	Model           string `toml:"model" json:"model"`
	DisplayName     string `toml:"display_name" json:"displayName"`
	BaseURL         string `toml:"base_url" json:"baseUrl"`
	APIKey          string `toml:"api_key" json:"apiKey"`
	Provider        string `toml:"provider" json:"provider"`
	MaxOutputTokens int    `toml:"max_output_tokens" json:"maxOutputTokens,omitempty"`
}

// Dep represents a dependency — either a simple URL string or a table with url + build.
type Dep struct {
	URL   string
	Build string
}

type Skills struct {
	Local  []string       `toml:"local"`
	GitHub []GitHubSkills `toml:"github"`
}

type GitHubSkills struct {
	URL     string   `toml:"url"`
	Include []string `toml:"include"`
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
	Platforms    []string          `toml:"platforms"`
	Instructions []string          `toml:"instructions"`
	Deps         map[string]any    `toml:"deps"`
	Skills       Skills            `toml:"skills"`
	Subagents    Subagents         `toml:"subagents"`
	MCP          map[string]MCP    `toml:"mcp"`
	Antigravity  AntigravityConfig `toml:"antigravity"`
	Droid        DroidConfig       `toml:"droid"`
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
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		var strictErr *toml.StrictMissingError
		if errors.As(err, &strictErr) {
			return nil, fmt.Errorf("parsing %s: %s", path, strictErr.String())
		}
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	deps, err := parseDeps(raw.Deps)
	if err != nil {
		return nil, fmt.Errorf("parsing deps in %s: %w", path, err)
	}

	cfg := &Config{
		Platforms:    raw.Platforms,
		Instructions: raw.Instructions,
		Deps:         deps,
		Skills:       raw.Skills,
		Subagents:    raw.Subagents,
		MCP:          raw.MCP,
		Antigravity:  raw.Antigravity,
		Droid:        raw.Droid,
	}
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating %s: %w", path, err)
	}

	return cfg, nil
}

type writableConfig struct {
	Platforms    []string               `toml:"platforms"`
	Instructions []string               `toml:"instructions,omitempty"`
	Deps         map[string]writableDep `toml:"deps,omitempty"`
	Skills       Skills                 `toml:"skills,omitempty"`
	Subagents    Subagents              `toml:"subagents,omitempty"`
	MCP          map[string]MCP         `toml:"mcp,omitempty"`
	Antigravity  AntigravityConfig      `toml:"antigravity,omitempty"`
	Droid        DroidConfig            `toml:"droid,omitempty"`
}

type writableDep struct {
	URL   string `toml:"url"`
	Build string `toml:"build,omitempty"`
}

func SaveAtomic(path string, cfg *Config) error {
	if err := validate(cfg); err != nil {
		return fmt.Errorf("validating config before write: %w", err)
	}
	deps := make(map[string]writableDep, len(cfg.Deps))
	for name, dep := range cfg.Deps {
		deps[name] = writableDep{URL: dep.URL, Build: dep.Build}
	}
	writable := writableConfig{
		Platforms: cfg.Platforms, Instructions: cfg.Instructions, Deps: deps,
		Skills: cfg.Skills, Subagents: cfg.Subagents, MCP: cfg.MCP,
		Antigravity: cfg.Antigravity, Droid: cfg.Droid,
	}
	var output bytes.Buffer
	encoder := toml.NewEncoder(&output).SetArraysMultiline(true)
	if err := encoder.Encode(writable); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".chai.toml.tmp-")
	if err != nil {
		return fmt.Errorf("creating temporary config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting temporary config permissions: %w", err)
	}
	if _, err := tmp.Write(output.Bytes()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temporary config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}
	return nil
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
			for field := range val {
				if field != "url" && field != "build" {
					return nil, fmt.Errorf("dep %q: unknown field %q", name, field)
				}
			}
			if url, ok := val["url"].(string); ok {
				d.URL = url
			} else {
				return nil, fmt.Errorf("dep %q: table requires a 'url' field", name)
			}
			if rawBuild, exists := val["build"]; exists {
				build, ok := rawBuild.(string)
				if !ok {
					return nil, fmt.Errorf("dep %q: 'build' must be a string", name)
				}
				d.Build = build
			}
			deps[name] = d
		default:
			return nil, fmt.Errorf("dep %q: must be a string or table, got %T", name, v)
		}
	}

	return deps, nil
}

func validate(cfg *Config) error {
	if len(cfg.Platforms) == 0 {
		return fmt.Errorf("platforms must contain at least one platform")
	}
	seenPlatforms := make(map[string]bool, len(cfg.Platforms))
	for _, platform := range cfg.Platforms {
		key := strings.ToLower(platform)
		if !platformpkg.IsSupported(platform) {
			return fmt.Errorf("unsupported platform %q", platform)
		}
		if seenPlatforms[key] {
			return fmt.Errorf("duplicate platform %q", platform)
		}
		seenPlatforms[key] = true
	}

	return validateSkills(cfg.Skills)
}

func validateSkills(skills Skills) error {
	seenLocal := make(map[string]bool, len(skills.Local))
	for i, local := range skills.Local {
		if strings.TrimSpace(local) == "" {
			return fmt.Errorf("skills.local[%d] must not be empty", i)
		}
		if strings.ContainsAny(local, "*?[]") {
			return fmt.Errorf("skills.local[%d] must not contain glob metacharacters", i)
		}
		if local != "~" && !strings.HasPrefix(local, "~/") && !filepath.IsAbs(local) &&
			!strings.HasPrefix(local, "./") && !strings.HasPrefix(local, "../") {
			return fmt.Errorf("skills.local[%d] must be an absolute, ~/, ./, or ../ local path", i)
		}
		cleaned := filepath.Clean(local)
		if seenLocal[cleaned] {
			return fmt.Errorf("duplicate local path %q", local)
		}
		seenLocal[cleaned] = true
	}

	seenRepositories := make(map[string]bool, len(skills.GitHub))
	var selectedSkills []skill.Source
	for i, source := range skills.GitHub {
		if _, err := githubskill.ParseCanonical(source.URL); err != nil {
			return fmt.Errorf("skills.github[%d].url must be a canonical https://github.com/owner/repo URL", i)
		}
		if seenRepositories[source.URL] {
			return fmt.Errorf("duplicate GitHub repository %q", source.URL)
		}
		seenRepositories[source.URL] = true
		if len(source.Include) == 0 {
			return fmt.Errorf("skills.github[%d].include must contain at least one skill name", i)
		}

		seenIncludes := make(map[string]bool, len(source.Include))
		for j, name := range source.Include {
			if !skill.ValidName(name) {
				return fmt.Errorf("invalid skill name %q at skills.github[%d].include[%d]", name, i, j)
			}
			if seenIncludes[name] {
				return fmt.Errorf("duplicate skill name %q in GitHub repository %q", name, source.URL)
			}
			seenIncludes[name] = true
			selectedSkills = append(selectedSkills, skill.Source{Name: name, Path: source.URL})
		}
	}

	return skill.ValidateUniqueNames(selectedSkills)
}
