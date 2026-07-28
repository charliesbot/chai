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
	skills, err := resolveSkillSources(skillPatterns, home)
	if err != nil {
		return err
	}

	if len(skills) == 0 {
		return nil
	}

	if dryRun {
		fmt.Println(ui.Label.Render("skills"))
		for _, src := range skills {
			fmt.Printf("  %s %s\n", ui.Muted.Render("source:"), src.path)
		}
	}

	status := newPlatformStatus(platforms)
	for _, p := range platforms {
		destDir := filepath.Join(home, p.SkillsDir)
		if dryRun {
			for _, src := range skills {
				fmt.Printf("  %s %s %s %s\n", ui.Arrow(), ui.Bold.Render(p.Name), ui.Muted.Render(filepath.Join(destDir, src.name)), ui.Muted.Render("→ "+src.path))
			}
		} else {
			if err := syncSkillCopies(skills, destDir, hashDB); err != nil {
				status.setFailed(p.Name)
			}
		}
	}

	if !dryRun {
		names := make([]string, len(skills))
		for i, s := range skills {
			names[i] = s.name
		}
		fmt.Println(ui.Box("skills", len(skills), status.statuses(), names))
	}

	if dryRun {
		fmt.Println()
	}

	return nil
}

type skillSource struct {
	path string
	name string
	kind skillSourceKind
}

type skillSourceKind int

const (
	skillSourceDir skillSourceKind = iota
	skillSourceFile
)

func resolveSkillSources(patterns []string, home string) ([]skillSource, error) {
	var all []skillSource
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		resolved, err := resolve.PathWithHome(pattern, home)
		if err != nil {
			return nil, fmt.Errorf("resolving path %q: %w", pattern, err)
		}

		info, statErr := os.Stat(resolved)
		if statErr == nil {
			if source, ok := skillSourceFromPath(resolved, info); ok {
				addSkillSource(&all, seen, source)
				continue
			}
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
			if err != nil {
				continue
			}
			if source, ok := skillSourceFromPath(m, info); ok {
				addSkillSource(&all, seen, source)
			}
		}
	}

	return all, nil
}

func skillSourceFromPath(path string, info os.FileInfo) (skillSource, bool) {
	if info.IsDir() {
		return skillSource{
			path: path,
			name: filepath.Base(path),
			kind: skillSourceDir,
		}, true
	}
	if isSkillMD(path) {
		return skillSource{
			path: path,
			name: filepath.Base(filepath.Dir(path)),
			kind: skillSourceFile,
		}, true
	}
	return skillSource{}, false
}

func addSkillSource(sources *[]skillSource, seen map[string]bool, source skillSource) {
	if seen[source.path] {
		return
	}
	seen[source.path] = true
	*sources = append(*sources, source)
}

func isSkillMD(path string) bool {
	return filepath.Base(path) == "SKILL.md"
}

func syncSkillCopies(sources []skillSource, destDir string, hashDB hash.DB) error {
	return syncNamedDirCopies(sources, destDir, hashDB, copySkillSource)
}

func syncNamedDirCopies(
	sources []skillSource,
	destDir string,
	hashDB hash.DB,
	copySource func(skillSource, string) (string, error),
) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	expected := make(map[string]bool)
	for _, src := range sources {
		expected[filepath.Join(destDir, src.name)] = true
	}
	if _, err := removeStaleManagedDirs(destDir, expected, hashDB); err != nil {
		return err
	}

	for _, src := range sources {
		dest := filepath.Join(destDir, src.name)

		if err := os.RemoveAll(dest); err != nil {
			return fmt.Errorf("removing %s: %w", dest, err)
		}

		sum, err := copySource(src, dest)
		if err != nil {
			return err
		}
		hashDB[dest] = sum
	}

	return nil
}

func copySkillSource(src skillSource, dest string) (string, error) {
	switch src.kind {
	case skillSourceFile:
		data, err := os.ReadFile(src.path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", src.path, err)
		}
		if err := atomicWrite(filepath.Join(dest, "SKILL.md"), data); err != nil {
			return "", err
		}
		return hash.Sum(data), nil
	case skillSourceDir:
		return copyAndHashDir(src.path, dest)
	default:
		return "", fmt.Errorf("unknown skill source kind for %s", src.path)
	}
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
	named := make([]skillSource, 0, len(sources))
	for _, src := range sources {
		named = append(named, skillSource{
			path: src,
			name: filepath.Base(src),
			kind: skillSourceDir,
		})
	}

	return syncNamedDirCopies(named, destDir, hashDB, func(src skillSource, dest string) (string, error) {
		return copyAndHashDir(src.path, dest)
	})
}

func copyAndHashDir(src, dest string) (string, error) {
	if err := copyTree(src, dest); err != nil {
		return "", fmt.Errorf("copying %s → %s: %w", src, dest, err)
	}
	sum, err := dirHash(src)
	if err != nil {
		return "", fmt.Errorf("hashing %s: %w", src, err)
	}
	return sum, nil
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
