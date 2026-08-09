package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/resolve"
	"github.com/charliesbot/chai/internal/ui"
	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type codexAgent struct {
	Name                  string `toml:"name"`
	Description           string `toml:"description"`
	DeveloperInstructions string `toml:"developer_instructions"`
}

// syncAgents resolves subagent patterns, then copies them to each platform's
// agents directory.
func syncAgents(agentPatterns []string, home string, platforms []platform.Platform, dryRun bool, hashDB hash.DB) error {
	agents, err := resolveFilePatterns(agentPatterns, home)
	if err != nil {
		return err
	}

	if len(agents) == 0 {
		return nil
	}

	if dryRun {
		fmt.Println(ui.Label.Render("subagents"))
		for _, src := range agents {
			fmt.Printf("  %s %s\n", ui.Muted.Render("source:"), src)
		}
	}

	status := newPlatformStatus(platforms)
	for _, p := range platforms {
		if p.Agents == nil {
			status.setNA(p.Name)
			if !dryRun {
				fmt.Printf("  %s %s %s\n", ui.Skip(), ui.Bold.Render(p.Name), ui.Muted.Render("subagents not supported — skipping"))
			}
			continue
		}
		destDir := filepath.Join(home, p.Agents.Dir)
		if dryRun {
			for _, src := range agents {
				name := filepath.Base(src)
				if p.Agents.Format == platform.AgentFormatCodexTOML {
					name = strings.TrimSuffix(name, filepath.Ext(name)) + ".toml"
				}
				fmt.Printf("  %s %s %s %s\n", ui.Arrow(), ui.Bold.Render(p.Name), ui.Muted.Render(filepath.Join(destDir, name)), ui.Muted.Render("→ "+src))
			}
		} else {
			var err error
			switch p.Agents.Format {
			case platform.AgentFormatCodexTOML:
				err = syncCodexAgentCopies(agents, destDir, hashDB)
			default:
				err = syncFileCopies(agents, destDir, hashDB)
			}
			if err != nil {
				status.setFailed(p.Name)
				return fmt.Errorf("syncing %s subagents: %w", p.Name, err)
			}
		}
	}

	if !dryRun {
		names := make([]string, len(agents))
		for i, s := range agents {
			names[i] = filepath.Base(s)
		}
		fmt.Println(ui.Box("subagents", len(agents), status.statuses(), names))
	}

	if dryRun {
		fmt.Println()
	}

	return nil
}

// resolveFilePatterns expands glob patterns and returns deduplicated absolute paths to .md files.
// Unlike resolvePatterns, which returns directories, this returns individual files.
func resolveFilePatterns(patterns []string, home string) ([]string, error) {
	var all []string
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		matches, err := resolve.GlobWithHome(pattern, home)
		if err != nil {
			return nil, fmt.Errorf("resolving pattern %q: %w", pattern, err)
		}
		for _, m := range matches {
			name := filepath.Base(m)
			if strings.HasPrefix(name, ".") {
				continue
			}
			if filepath.Ext(m) != ".md" {
				continue
			}
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			if !seen[m] {
				seen[m] = true
				all = append(all, m)
			}
		}
	}

	return all, nil
}

func syncCodexAgentCopies(sources []string, destDir string, hashDB hash.DB) error {
	generated := make(map[string][]byte, len(sources))
	for _, src := range sources {
		data, err := compileCodexAgent(src)
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)) + ".toml"
		generated[filepath.Join(destDir, name)] = data
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	expected := make(map[string]bool, len(generated))
	for dest := range generated {
		expected[dest] = true
	}
	if err := removeStaleManagedFiles(destDir, ".toml", expected, hashDB); err != nil {
		return err
	}

	for dest, data := range generated {
		if err := writeManagedFile(dest, data, hashDB); err != nil {
			return err
		}
	}

	return nil
}

func compileCodexAgent(src string) ([]byte, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", src, err)
	}

	frontmatter, body, err := splitFrontmatter(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", src, err)
	}

	var meta struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
		return nil, fmt.Errorf("%s: parsing frontmatter: %w", src, err)
	}

	agent := codexAgent{
		Description:           strings.TrimSpace(meta.Description),
		DeveloperInstructions: strings.TrimSpace(body),
		Name:                  strings.TrimSpace(meta.Name),
	}
	if agent.Name == "" {
		return nil, fmt.Errorf("%s: missing required frontmatter field: name", src)
	}
	if agent.Description == "" {
		return nil, fmt.Errorf("%s: missing required frontmatter field: description", src)
	}
	if agent.DeveloperInstructions == "" {
		return nil, fmt.Errorf("%s: missing required body for developer_instructions", src)
	}

	out, err := toml.Marshal(agent)
	if err != nil {
		return nil, fmt.Errorf("%s: marshaling Codex agent TOML: %w", src, err)
	}
	return append(out, '\n'), nil
}

func splitFrontmatter(data string) (string, string, error) {
	data = strings.TrimPrefix(data, "\ufeff")
	data = strings.ReplaceAll(data, "\r\n", "\n")
	if !strings.HasPrefix(data, "---\n") {
		return "", "", fmt.Errorf("missing YAML frontmatter")
	}
	rest := strings.TrimPrefix(data, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", fmt.Errorf("unterminated YAML frontmatter")
	}
	frontmatter := rest[:end]
	body := rest[end+len("\n---"):]
	body = strings.TrimPrefix(body, "\r\n")
	body = strings.TrimPrefix(body, "\n")
	return frontmatter, body, nil
}
