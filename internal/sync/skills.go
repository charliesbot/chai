package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/resolve"
	"github.com/charliesbot/chai/internal/ui"
)

// syncSkills symlinks skill directories to each platform's skills directory.
func syncSkills(patterns []string, home string, dryRun bool) error {
	if len(patterns) == 0 {
		return nil
	}

	sources, err := resolvePatterns(patterns, home)
	if err != nil {
		return err
	}

	if dryRun && len(sources) > 0 {
		fmt.Println(ui.Label.Render("skills"))
		for _, src := range sources {
			fmt.Printf("  %s %s\n", ui.Muted.Render("source:"), src)
		}
	}

	platforms := platform.All()
	for _, p := range platforms {
		destDir := filepath.Join(home, p.SkillsDir)

		if dryRun {
			for _, src := range sources {
				name := filepath.Base(src)
				dest := filepath.Join(destDir, name)
				fmt.Printf("  %s %s %s %s\n", ui.Arrow(), ui.Bold.Render(p.Name), ui.Muted.Render(dest), ui.Muted.Render("→ "+src))
			}
			continue
		}

		if err := syncSymlinks(sources, destDir); err != nil {
			return fmt.Errorf("syncing skills to %s: %w", p.Name, err)
		}
		for _, src := range sources {
			name := filepath.Base(src)
			fmt.Println(ui.SyncedLine(p.Name, filepath.Join(destDir, name)))
		}
	}

	if dryRun && len(sources) > 0 {
		fmt.Println()
	}

	return nil
}

// syncAgents symlinks agent directories to each platform's skills directory.
// Agents go in the same skills directory as skills.
func syncAgents(patterns []string, home string, dryRun bool) error {
	if len(patterns) == 0 {
		return nil
	}

	sources, err := resolvePatterns(patterns, home)
	if err != nil {
		return err
	}

	if dryRun && len(sources) > 0 {
		fmt.Println(ui.Label.Render("agents"))
		for _, src := range sources {
			fmt.Printf("  %s %s\n", ui.Muted.Render("source:"), src)
		}
	}

	platforms := platform.All()
	for _, p := range platforms {
		destDir := filepath.Join(home, p.SkillsDir)

		if dryRun {
			for _, src := range sources {
				name := filepath.Base(src)
				dest := filepath.Join(destDir, name)
				fmt.Printf("  %s %s %s %s\n", ui.Arrow(), ui.Bold.Render(p.Name), ui.Muted.Render(dest), ui.Muted.Render("→ "+src))
			}
			continue
		}

		if err := syncSymlinks(sources, destDir); err != nil {
			return fmt.Errorf("syncing agents to %s: %w", p.Name, err)
		}
		for _, src := range sources {
			name := filepath.Base(src)
			fmt.Println(ui.SyncedLine(p.Name, filepath.Join(destDir, name)))
		}
	}

	if dryRun && len(sources) > 0 {
		fmt.Println()
	}

	return nil
}

// resolvePatterns expands all glob patterns and returns deduplicated absolute paths.
func resolvePatterns(patterns []string, home string) ([]string, error) {
	var all []string
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		// If the path is a directory (no glob chars), treat it as dir/*
		resolved, err := resolve.PathWithHome(pattern, home)
		if err != nil {
			return nil, fmt.Errorf("resolving path %q: %w", pattern, err)
		}
		info, statErr := os.Stat(resolved)
		if statErr == nil && info.IsDir() {
			pattern = pattern + "/*"
		}

		matches, err := resolve.GlobWithHome(pattern, home)
		if err != nil {
			return nil, fmt.Errorf("resolving pattern %q: %w", pattern, err)
		}
		for _, m := range matches {
			name := filepath.Base(m)
			if strings.HasPrefix(name, ".") {
				continue
			}
			info, err := os.Stat(m)
			if err != nil || !info.IsDir() {
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

// syncSymlinks creates symlinks in destDir for each source directory.
// Removes existing symlinks in destDir that were managed by chai (are symlinks)
// before creating new ones.
func syncSymlinks(sources []string, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	// Build set of expected symlink names
	expected := make(map[string]bool, len(sources))
	for _, src := range sources {
		expected[filepath.Base(src)] = true
	}

	// Remove stale symlinks (only symlinks, not regular files/dirs)
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", destDir, err)
	}
	for _, entry := range entries {
		path := filepath.Join(destDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 && !expected[entry.Name()] {
			os.Remove(path)
		}
	}

	// Create symlinks
	for _, src := range sources {
		name := filepath.Base(src)
		dest := filepath.Join(destDir, name)

		// Remove existing symlink if it exists (might point to old location)
		if info, err := os.Lstat(dest); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				os.Remove(dest)
			} else {
				return fmt.Errorf("%s exists and is not a symlink — refusing to overwrite", dest)
			}
		}

		if err := os.Symlink(src, dest); err != nil {
			return fmt.Errorf("symlinking %s → %s: %w", dest, src, err)
		}
	}

	return nil
}
