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
	"github.com/charliesbot/chai/internal/resolve"
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
	if err := rejectSourceOverlaps(cfg, home, targets.dirs); err != nil {
		return err
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
		dirs := []string{p.SkillsDir}
		if p.Agents != nil {
			dirs = append(dirs, p.Agents.Dir)
		}
		for _, dir := range dirs {
			if dir == "" {
				continue
			}
			path := filepath.Join(home, dir)
			if !seenDirs[path] {
				seenDirs[path] = true
				out.dirs = append(out.dirs, path)
			}
		}

		if p.MCP == nil {
			continue
		}
		target := mcpTarget{
			path:   filepath.Join(home, p.MCP.ConfigPath),
			key:    p.MCP.Key,
			format: p.MCP.Format,
		}
		if !seenMCPs[target] {
			seenMCPs[target] = true
			out.mcps = append(out.mcps, target)
		}
	}

	return out
}

func rejectSourceOverlaps(cfg *config.Config, home string, dirs []string) error {
	patterns := make([]string, 0, len(cfg.Skills.Paths)+len(cfg.Subagents.Paths))
	patterns = append(patterns, cfg.Skills.Paths...)
	patterns = append(patterns, cfg.Subagents.Paths...)

	sources, err := configuredSourcePaths(home, patterns)
	if err != nil {
		return err
	}
	cleanDirs := expandedPaths(dirs)
	sourcePaths := expandedPaths(sources)

	for _, dir := range cleanDirs {
		for _, source := range sourcePaths {
			if pathsOverlap(dir, source) {
				return fmt.Errorf("refusing to clean %s because it overlaps configured source path %s", dir, source)
			}
		}
	}
	return nil
}

func configuredSourcePaths(home string, patterns []string) ([]string, error) {
	seen := make(map[string]bool)
	var sources []string

	for _, pattern := range patterns {
		resolved, err := resolve.PathWithHome(pattern, home)
		if err != nil {
			return nil, fmt.Errorf("resolving source path %q: %w", pattern, err)
		}
		resolved = absPath(home, resolved)

		if !hasGlobMeta(resolved) {
			if !seen[resolved] {
				seen[resolved] = true
				sources = append(sources, resolved)
			}
			continue
		}

		base := globBase(resolved)
		if base != "" && !seen[base] {
			seen[base] = true
			sources = append(sources, base)
		}

		matches, err := filepath.Glob(resolved)
		if err != nil {
			return nil, fmt.Errorf("resolving source pattern %q: %w", pattern, err)
		}
		for _, match := range matches {
			if !seen[match] {
				seen[match] = true
				sources = append(sources, match)
			}
		}
	}

	return sources, nil
}

func expandedPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		addPath(&out, seen, filepath.Clean(path))
		if real, err := filepath.EvalSymlinks(path); err == nil {
			addPath(&out, seen, filepath.Clean(real))
		}
	}
	return out
}

func addPath(paths *[]string, seen map[string]bool, path string) {
	if seen[path] {
		return
	}
	seen[path] = true
	*paths = append(*paths, path)
}

func absPath(home, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(home, path)
}

func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func globBase(pattern string) string {
	volume := filepath.VolumeName(pattern)
	rest := strings.TrimPrefix(pattern, volume)
	rooted := strings.HasPrefix(rest, string(filepath.Separator))
	rest = strings.TrimPrefix(rest, string(filepath.Separator))

	parts := strings.Split(rest, string(filepath.Separator))
	for i, part := range parts {
		if hasGlobMeta(part) {
			if i == 0 {
				if rooted {
					return volume + string(filepath.Separator)
				}
				return volume
			}
			base := filepath.Join(parts[:i]...)
			if rooted {
				base = string(filepath.Separator) + base
			}
			return volume + base
		}
	}
	return pattern
}

func pathsOverlap(a, b string) bool {
	if pathWithin(a, b) || pathWithin(b, a) {
		return true
	}
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	return pathWithin(a, b) || pathWithin(b, a)
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
