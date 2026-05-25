package clean

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/ui"
)

// Options controls clean behavior.
type Options struct {
	DryRun bool
}

// Run removes generated platform outputs for the configured platforms.
func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	return RunWithHome(ctx, cfg, home, opts)
}

// RunWithHome removes generated platform outputs using the given home directory.
func RunWithHome(ctx context.Context, cfg *config.Config, home string, opts Options) error {
	if opts.DryRun {
		fmt.Println(ui.DryRunTag() + " " + ui.Muted.Render("previewing clean — no files will be deleted"))
		fmt.Println()
	}

	platforms := platform.ForNames(cfg.Platforms)
	targets := cleanTargets(home, platforms)
	if len(targets.dirs) == 0 && len(targets.mcps) == 0 {
		fmt.Println(ui.Muted.Render("nothing to clean"))
		return nil
	}

	if opts.DryRun {
		printDryRun(targets)
		return nil
	}

	hashDB, err := hash.Load(home)
	if err != nil {
		return err
	}

	if err := removeDirs(ctx, targets.dirs, hashDB); err != nil {
		return err
	}
	if err := removeMCPKeys(ctx, targets.mcps); err != nil {
		return err
	}
	if err := hashDB.Save(home); err != nil {
		return err
	}

	fmt.Println(ui.Box("clean", len(targets.dirs)+len(targets.mcps), cleanStatuses(platforms), cleanItems(targets)))
	return nil
}

type mcpTarget struct {
	path   string
	key    string
	format platform.MCPFormat
}

type targets struct {
	dirs []string
	mcps []mcpTarget
}

func cleanTargets(home string, platforms []platform.Platform) targets {
	var out targets
	seenDirs := make(map[string]bool)
	seenMCPs := make(map[mcpTarget]bool)

	for _, p := range platforms {
		for _, dir := range []string{p.SkillsDir, p.AgentsDir} {
			if dir == "" {
				continue
			}
			path := filepath.Join(home, dir)
			if !seenDirs[path] {
				seenDirs[path] = true
				out.dirs = append(out.dirs, path)
			}
		}

		target := mcpTarget{
			path:   filepath.Join(home, p.MCPConfigPath),
			key:    p.MCPKey,
			format: p.MCPFormat,
		}
		if !seenMCPs[target] {
			seenMCPs[target] = true
			out.mcps = append(out.mcps, target)
		}
	}

	return out
}

func printDryRun(targets targets) {
	if len(targets.dirs) > 0 {
		fmt.Println(ui.Label.Render("directories"))
		for _, dir := range targets.dirs {
			fmt.Printf("  %s %s\n", ui.Arrow(), ui.Muted.Render(dir))
		}
		fmt.Println()
	}

	if len(targets.mcps) > 0 {
		fmt.Println(ui.Label.Render("mcp keys"))
		for _, target := range targets.mcps {
			fmt.Printf("  %s %s %s\n", ui.Arrow(), ui.Muted.Render(target.path), ui.Muted.Render("remove "+target.key))
		}
		fmt.Println()
	}
}

func removeDirs(ctx context.Context, dirs []string, hashDB hash.DB) error {
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("clean interrupted: %w", err)
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("removing %s: %w", dir, err)
		}
		removeHashEntriesUnder(hashDB, dir)
	}
	return nil
}

func removeHashEntriesUnder(hashDB hash.DB, root string) {
	for path := range hashDB {
		if pathWithin(root, path) {
			delete(hashDB, path)
		}
	}
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func removeMCPKeys(ctx context.Context, targets []mcpTarget) error {
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("clean interrupted: %w", err)
		}
		var err error
		if target.format == platform.MCPFormatCodex {
			err = removeTOMLKey(target.path, target.key)
		} else {
			err = removeJSONKey(target.path, target.key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func removeJSONKey(path, key string) error {
	existing := make(map[string]any)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &existing); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if _, ok := existing[key]; !ok {
		return nil
	}
	delete(existing, key)

	out, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", path, err)
	}
	out = append(out, '\n')
	return atomicWrite(path, out)
}

func removeTOMLKey(path, key string) error {
	existing := make(map[string]any)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := toml.Unmarshal(data, &existing); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if _, ok := existing[key]; !ok {
		return nil
	}
	delete(existing, key)

	out, err := toml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", path, err)
	}
	return atomicWrite(path, out)
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming %s → %s: %w", tmp, path, err)
	}
	return nil
}

func cleanStatuses(platforms []platform.Platform) []ui.PlatformStatus {
	seen := make(map[string]bool)
	statuses := make([]ui.PlatformStatus, 0, len(platforms))
	for _, p := range platforms {
		name := p.Name
		if p.Key == "antigravity" {
			name = "Antigravity"
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		statuses = append(statuses, ui.PlatformStatus{Name: name, State: ui.PlatformOK})
	}
	return statuses
}

func cleanItems(targets targets) []string {
	items := make([]string, 0, len(targets.dirs)+len(targets.mcps))
	for _, dir := range targets.dirs {
		items = append(items, "rm "+dir)
	}
	for _, target := range targets.mcps {
		items = append(items, "remove "+target.key+" from "+target.path)
	}
	return items
}
