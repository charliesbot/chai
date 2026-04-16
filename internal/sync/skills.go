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

// syncSkillsAndAgents resolves skills and agents patterns, then symlinks
// them to each platform's respective directories.
func syncSkillsAndAgents(skillPatterns, agentPatterns []string, home string, platforms []platform.Platform, dryRun bool, hashDB hash.DB) error {
	skills, err := resolvePatterns(skillPatterns, home)
	if err != nil {
		return err
	}
	agents, err := resolveFilePatterns(agentPatterns, home)
	if err != nil {
		return err
	}

	if len(skills) == 0 && len(agents) == 0 {
		return nil
	}

	if dryRun {
		if len(skills) > 0 {
			fmt.Println(ui.Label.Render("skills"))
			for _, src := range skills {
				fmt.Printf("  %s %s\n", ui.Muted.Render("source:"), src)
			}
		}
		if len(agents) > 0 {
			fmt.Println(ui.Label.Render("subagents"))
			for _, src := range agents {
				fmt.Printf("  %s %s\n", ui.Muted.Render("source:"), src)
			}
		}
	}

	// Skills
	if len(skills) > 0 {
		status := newPlatformStatus(platforms)
		for _, p := range platforms {
			destDir := filepath.Join(home, p.SkillsDir)
			if dryRun {
				for _, src := range skills {
					name := filepath.Base(src)
					fmt.Printf("  %s %s %s %s\n", ui.Arrow(), ui.Bold.Render(p.Name), ui.Muted.Render(filepath.Join(destDir, name)), ui.Muted.Render("→ "+src))
				}
			} else {
				if err := syncSymlinks(skills, destDir); err != nil {
					status.setFailed(p.Name)
				}
			}
		}
		if !dryRun {
			names := make([]string, len(skills))
			for i, s := range skills {
				names[i] = filepath.Base(s)
			}
			fmt.Println(ui.Box("skills", len(skills), status.claude(), status.gemini(), names))
		}
	}

	// Subagents
	if len(agents) > 0 {
		status := newPlatformStatus(platforms)
		for _, p := range platforms {
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
			fmt.Println(ui.Box("subagents", len(agents), status.claude(), status.gemini(), names))
		}
	}

	if dryRun && (len(skills) > 0 || len(agents) > 0) {
		fmt.Println()
	}

	return nil
}

// resolveFilePatterns expands glob patterns and returns deduplicated absolute paths to .md files.
// Unlike resolvePatterns (which returns directories), this returns individual files.
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

// resolvePatterns expands all glob patterns and returns deduplicated absolute paths.
func resolvePatterns(patterns []string, home string) ([]string, error) {
	var all []string
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		resolved, err := resolve.PathWithHome(pattern, home)
		if err != nil {
			return nil, fmt.Errorf("resolving path %q: %w", pattern, err)
		}

		// If the resolved path is an existing directory (no glob), include it directly.
		info, statErr := os.Stat(resolved)
		if statErr == nil && info.IsDir() {
			if !seen[resolved] {
				seen[resolved] = true
				all = append(all, resolved)
			}
			continue
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

// syncFileCopies copies source files into destDir.
// Uses the hash DB to track which files chai manages:
//   - Stale chai-managed files (in hash DB but not in sources) are removed.
//   - User-created files (not in hash DB and not in sources) are left alone.
//
// Uses atomic writes (write to .tmp, then rename).
func syncFileCopies(sources []string, destDir string, hashDB hash.DB) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	// Build set of expected destination paths
	expected := make(map[string]bool, len(sources))
	for _, src := range sources {
		dest := filepath.Join(destDir, filepath.Base(src))
		expected[dest] = true
	}

	// Remove stale chai-managed files, warn about user-created ones
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", destDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(destDir, entry.Name())
		if expected[path] {
			continue
		}
		if _, managed := hashDB[path]; managed {
			// Chai put this here previously, now it's gone from sources — remove
			os.Remove(path)
			delete(hashDB, path)
		} else {
			fmt.Printf("  %s %s %s\n", ui.Warning.Render("!"), entry.Name(), ui.Muted.Render("not managed by chai — skipping"))
		}
	}

	// Copy files atomically and update hashes
	for _, src := range sources {
		name := filepath.Base(src)
		dest := filepath.Join(destDir, name)

		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("reading %s: %w", src, err)
		}

		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", tmp, err)
		}
		if err := os.Rename(tmp, dest); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("renaming %s → %s: %w", tmp, dest, err)
		}

		hashDB[dest] = hash.Sum(data)
	}

	return nil
}

// syncSymlinks creates symlinks in destDir for each source directory.
// Removes stale symlinks (managed by chai) before creating new ones.
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
