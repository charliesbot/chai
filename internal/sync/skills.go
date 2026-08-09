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
	"github.com/charliesbot/chai/internal/skill"
	"github.com/charliesbot/chai/internal/ui"
)

func resolveLocalSkillSources(roots []string, baseDir, home string) ([]skill.Source, error) {
	return skill.DiscoverLocal(roots, baseDir, home)
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
		details := changes.details()
		const detailLimit = 5
		for i, detail := range details {
			if i == detailLimit {
				fmt.Printf("   %s %s\n", ui.Muted.Render("..."), ui.ItemStyle.Render(fmt.Sprintf("%d more", len(details)-detailLimit)))
				break
			}
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
	changes := newItemChanges()
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return changes, fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	expected := make(map[string]bool)
	for _, src := range sources {
		expected[filepath.Join(destDir, src.Name)] = true
	}
	stale, err := removeStaleManagedDirs(destDir, expected, hashDB, opts)
	for _, name := range stale.removed {
		changes.record(name, itemRemoved)
	}
	for _, name := range stale.preserved {
		changes.preserved[name] = true
	}
	if err != nil {
		return changes, err
	}

	for _, src := range sources {
		dest := filepath.Join(destDir, src.Name)
		previousHash, managed := hashDB[dest]
		_, statErr := os.Stat(dest)
		existed := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return changes, fmt.Errorf("checking %s: %w", dest, statErr)
		}
		if existed && !managed {
			return changes, fmt.Errorf("skill destination %s is not managed by chai", dest)
		}
		if existed && managed && !opts.Force {
			dirty, err := managedDirDirty(dest, previousHash)
			if err != nil {
				return changes, err
			}
			if dirty {
				if opts.Prompt == nil {
					return changes, &DirtyError{Files: []string{dest}}
				}
				overwrite, err := opts.Prompt(dest)
				if err != nil {
					return changes, err
				}
				if !overwrite {
					return changes, fmt.Errorf("skill sync incomplete: modified destination %s was preserved", dest)
				}
			}
		}

		staging, err := os.MkdirTemp(destDir, "."+src.Name+".tmp-")
		if err != nil {
			return changes, fmt.Errorf("creating staging directory for %s: %w", dest, err)
		}
		sum, err := copyAndHashDir(src.Path, staging)
		if err != nil {
			_ = os.RemoveAll(staging)
			return changes, err
		}
		if err := replaceDirectory(staging, dest, existed); err != nil {
			return changes, err
		}
		hashDB[dest] = sum
		switch {
		case !managed:
			changes.record(src.Name, itemAdded)
		case !existed || previousHash != sum:
			changes.record(src.Name, itemUpdated)
		default:
			changes.record(src.Name, itemUnchanged)
		}
	}

	return changes, nil
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

func managedDirDirty(path, stored string) (bool, error) {
	current, err := dirHash(path)
	if err != nil {
		return false, fmt.Errorf("hashing managed directory %s: %w", path, err)
	}
	return current != stored, nil
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
