package sync

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/resolve"
	"github.com/charliesbot/chai/internal/ui"
)

// syncSkills resolves skill patterns, then copies them to each platform's
// skills directory.
func syncSkills(skillPatterns []string, home string, platforms []platform.Platform, dryRun bool, hashDB hash.DB) error {
	skills, err := resolvePatterns(skillPatterns, home)
	if err != nil {
		return err
	}

	if len(skills) == 0 {
		return nil
	}

	if dryRun {
		fmt.Println(ui.Label.Render("skills"))
		for _, src := range skills {
			fmt.Printf("  %s %s\n", ui.Muted.Render("source:"), src)
		}
	}

	status := newPlatformStatus(platforms)
	for _, p := range platforms {
		destDir := filepath.Join(home, p.SkillsDir)
		if dryRun {
			for _, src := range skills {
				name := filepath.Base(src)
				fmt.Printf("  %s %s %s %s\n", ui.Arrow(), ui.Bold.Render(p.Name), ui.Muted.Render(filepath.Join(destDir, name)), ui.Muted.Render("→ "+src))
			}
		} else {
			if err := syncDirCopies(skills, destDir, hashDB); err != nil {
				status.setFailed(p.Name)
			}
		}
	}

	if !dryRun {
		names := make([]string, len(skills))
		for i, s := range skills {
			names[i] = filepath.Base(s)
		}
		fmt.Println(ui.Box("skills", len(skills), status.statuses(), names))
	}

	if dryRun {
		fmt.Println()
	}

	return nil
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

	expected := expectedDestinations(sources, destDir, filepath.Base)
	if err := removeStaleManagedFiles(destDir, ".md", expected, hashDB); err != nil {
		return err
	}

	// Copy files atomically and update hashes
	for _, src := range sources {
		name := filepath.Base(src)
		dest := filepath.Join(destDir, name)

		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("reading %s: %w", src, err)
		}

		if err := writeManagedFile(dest, data, 0644, hashDB); err != nil {
			return err
		}
	}

	return nil
}

// syncDirCopies recursively copies source directories into destDir.
// Each source skill directory is wiped and re-copied on every sync so the
// destination mirrors the source exactly.
//
// Uses the hash DB to track which directories chai manages:
//   - Stale chai-managed directories (in hash DB but not in sources) are removed.
//   - User-created directories (not in hash DB and not in sources) are left alone.
//
// The hash stored per skill is a composite md5 of all files inside the source tree.
func syncDirCopies(sources []string, destDir string, hashDB hash.DB) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	expected := expectedDestinations(sources, destDir, filepath.Base)
	if err := removeStaleManagedDirs(destDir, expected, hashDB); err != nil {
		return err
	}

	for _, src := range sources {
		name := filepath.Base(src)
		dest := filepath.Join(destDir, name)

		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("removing %s: %w", dest, err)
		}
		if err := copyTree(src, dest); err != nil {
			return fmt.Errorf("copying %s → %s: %w", src, dest, err)
		}

		sum, err := dirHash(src)
		if err != nil {
			return fmt.Errorf("hashing %s: %w", src, err)
		}
		hashDB[dest] = sum
	}

	return nil
}

// copyTree recursively copies src into dst. Regular files are written atomically
// (temp file + rename). Symlinks inside src are skipped.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tmp := target + ".tmp"
		if err := os.WriteFile(tmp, data, info.Mode().Perm()); err != nil {
			return err
		}
		if err := os.Rename(tmp, target); err != nil {
			os.Remove(tmp)
			return err
		}
		return nil
	})
}

// dirHash computes a composite md5 over the directory's contents.
// It hashes "relPath\tmd5(content)" lines joined by newline, in sorted order,
// so the result is deterministic and changes when any file in the tree changes.
func dirHash(root string) (string, error) {
	var lines []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%s\t%s", rel, hash.Sum(data)))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(lines)
	return hash.Sum([]byte(strings.Join(lines, "\n"))), nil
}
