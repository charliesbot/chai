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
)

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
		if p.AgentsDir == "" {
			status.setNA(p.Name)
			if !dryRun {
				fmt.Printf("  %s %s %s\n", ui.Skip(), ui.Bold.Render(p.Name), ui.Muted.Render("subagents not supported — skipping"))
			}
			continue
		}
		destDir := filepath.Join(home, p.AgentsDir)
		if dryRun {
			for _, src := range agents {
				name := filepath.Base(src)
				fmt.Printf("  %s %s %s %s\n", ui.Arrow(), ui.Bold.Render(p.Name), ui.Muted.Render(filepath.Join(destDir, name)), ui.Muted.Render("→ "+src))
			}
		} else {
			if err := syncFileCopies(agents, destDir, hashDB); err != nil {
				status.setFailed(p.Name)
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
