package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/skill"
	"github.com/charliesbot/chai/internal/ui"
)

func resolveLocalSkillSources(roots []string, baseDir, home string) ([]skill.Source, error) {
	return skill.DiscoverLocal(roots, baseDir, home)
}

// ValidateUnmanagedSkillDestinations reports existing selected-skill paths that
// chai cannot safely overwrite because they are absent from its ownership DB.
func ValidateUnmanagedSkillDestinations(names []string, home string, platformNames []string) error {
	hashDB, err := hash.Load(home)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	var collisions []string
	for _, p := range platform.ForNames(platformNames) {
		for _, name := range names {
			path := filepath.Join(home, p.SkillsDir, name)
			if seen[path] {
				continue
			}
			seen[path] = true
			if _, managed := hashDB[path]; managed {
				continue
			}
			if _, err := os.Stat(path); err == nil {
				collisions = append(collisions, path)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("checking skill destination %s: %w", path, err)
			}
		}
	}
	if len(collisions) == 0 {
		return nil
	}
	sort.Strings(collisions)
	return fmt.Errorf(
		"existing skill destinations are not managed by chai:\n  %s\nmove or remove these directories, then retry",
		strings.Join(collisions, "\n  "),
	)
}

func syncResolvedSkills(skills []skill.Source, home string, platforms []platform.Platform, opts Options, hashDB hash.DB) error {
	if opts.DryRun {
		fmt.Println(ui.Label.Render("skills"))
		for _, src := range skills {
			fmt.Printf("  %s %s\n", ui.Muted.Render("source:"), src.Path)
		}
	}

	status := newPlatformStatus(platforms)
	changes := newItemChanges()
	var syncErr error
	for _, p := range platforms {
		destDir := filepath.Join(home, p.SkillsDir)
		if opts.DryRun {
			for _, src := range skills {
				fmt.Printf("  %s %s %s %s\n", ui.Arrow(), ui.Bold.Render(p.Name), ui.Muted.Render(filepath.Join(destDir, src.Name)), ui.Muted.Render("→ "+src.Path))
			}
		} else {
			platformChanges, err := syncSkillCopies(skills, destDir, hashDB, opts)
			changes.merge(platformChanges)
			if err != nil {
				status.setFailed(p.Name)
				if syncErr == nil {
					syncErr = err
				}
				continue
			}
		}
	}

	if !opts.DryRun {
		fmt.Println(ui.ResultLine("skills", changes.summary(), status.statuses()))
		for _, detail := range changes.details() {
			fmt.Printf("   %s\n", detail.render())
		}
		if count := len(changes.preserved); count > 0 {
			fmt.Printf(" %s %s\n", ui.Warning.Render("!"), ui.Muted.Render(fmt.Sprintf("%d unmanaged %s preserved", count, pluralize("skill", count))))
		}
	}

	if opts.DryRun {
		fmt.Println()
	}

	return syncErr
}

type itemChanges struct {
	items     map[string]itemChange
	preserved map[string]bool
}

type itemChange int

const (
	itemUnchanged itemChange = iota
	itemRemoved
	itemUpdated
	itemAdded
)

type itemChangeDetail struct {
	symbol string
	name   string
	change itemChange
}

func (detail itemChangeDetail) render() string {
	return detail.change.render(detail.symbol + " " + detail.name)
}

func newItemChanges() itemChanges {
	return itemChanges{
		items:     make(map[string]itemChange),
		preserved: make(map[string]bool),
	}
}

func (c *itemChanges) merge(other itemChanges) {
	for name, change := range other.items {
		c.record(name, change)
	}
	for name := range other.preserved {
		c.preserved[name] = true
	}
}

func (c *itemChanges) record(name string, change itemChange) {
	current, exists := c.items[name]
	if !exists || change > current {
		c.items[name] = change
	}
}

func (change itemChange) render(text string) string {
	switch change {
	case itemAdded:
		return ui.Added.Render(text)
	case itemUpdated:
		return ui.Updated.Render(text)
	case itemRemoved:
		return ui.Removed.Render(text)
	default:
		return ui.Muted.Render(text)
	}
}

func (c itemChanges) summary() string {
	counts := make(map[itemChange]int)
	for _, change := range c.items {
		counts[change]++
	}
	parts := make([]string, 0, 4)
	for _, item := range []struct {
		change itemChange
		label  string
	}{
		{itemAdded, "added"},
		{itemUpdated, "updated"},
		{itemRemoved, "removed"},
		{itemUnchanged, "unchanged"},
	} {
		if count := counts[item.change]; count > 0 {
			parts = append(parts, item.change.render(fmt.Sprintf("%d %s", count, item.label)))
		}
	}
	return strings.Join(parts, " · ")
}

func (c itemChanges) details() []itemChangeDetail {
	var details []itemChangeDetail
	for _, category := range []struct {
		symbol string
		change itemChange
	}{
		{"+", itemAdded},
		{"~", itemUpdated},
		{"-", itemRemoved},
	} {
		var names []string
		for name, change := range c.items {
			if change == category.change {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			details = append(details, itemChangeDetail{symbol: category.symbol, name: name, change: category.change})
		}
	}
	return details
}

func pluralize(noun string, count int) string {
	if count == 1 {
		return noun
	}
	return noun + "s"
}

func syncSkillCopies(sources []skill.Source, destDir string, hashDB hash.DB, opts Options) (itemChanges, error) {
	desired := make([]managedDesired, len(sources))
	for i, source := range sources {
		desired[i] = managedDesired{Name: source.Name, Source: source.Path}
	}
	result, err := reconcileManagedDirectories(destDir, desired, hashDB, skillReconciliationPolicy(opts))
	for _, name := range result.preserved {
		result.changes.preserved[name] = true
	}
	if err != nil {
		var unmanaged *unmanagedDestinationError
		if errors.As(err, &unmanaged) {
			return result.changes, fmt.Errorf("skill destination %s is not managed by chai", unmanaged.Path)
		}
		var declined *declinedDestinationError
		if errors.As(err, &declined) {
			if declined.Stale {
				return result.changes, fmt.Errorf("skill sync incomplete: modified stale destination %s was preserved", declined.Path)
			}
			return result.changes, fmt.Errorf("skill sync incomplete: modified destination %s was preserved", declined.Path)
		}
		return result.changes, err
	}
	return result.changes, nil
}

// syncFileCopies copies source files into destDir.
// Uses the hash DB to track which files chai manages:
//   - Stale chai-managed files (in hash DB but not in sources) are removed.
//   - User-created files (not in hash DB and not in sources) are left alone.
//
// Uses atomic writes (write to .tmp, then rename).
func syncFileCopies(sources []string, destDir string, hashDB hash.DB) error {
	desired := make([]managedDesired, len(sources))
	for i, source := range sources {
		desired[i] = managedDesired{Name: filepath.Base(source), Source: source}
	}
	result, err := reconcileManagedFiles(destDir, ".md", desired, hashDB, generatedFileReconciliationPolicy())
	if err != nil {
		return err
	}
	for _, name := range result.preserved {
		fmt.Printf("  %s %s %s\n", ui.Warning.Render("!"), name, ui.Muted.Render("not managed by chai — skipping"))
	}
	return nil
}

func replaceDirectory(staging, dest string, existed bool) error {
	backup := dest + ".backup"
	if err := os.RemoveAll(backup); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("removing old backup %s: %w", backup, err)
	}
	if existed {
		if err := os.Rename(dest, backup); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("staging existing destination %s: %w", dest, err)
		}
	}
	if err := os.Rename(staging, dest); err != nil {
		if existed {
			_ = os.Rename(backup, dest)
		}
		_ = os.RemoveAll(staging)
		return fmt.Errorf("promoting skill destination %s: %w", dest, err)
	}
	_ = os.RemoveAll(backup)
	return nil
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
