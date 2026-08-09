package sync

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/ui"
)

func expectedDestinations(sources []string, destDir string, destName func(string) string) map[string]bool {
	expected := make(map[string]bool, len(sources))
	for _, src := range sources {
		expected[filepath.Join(destDir, destName(src))] = true
	}
	return expected
}

func removeStaleManagedFiles(destDir, ext string, expected map[string]bool, hashDB hash.DB) error {
	result, err := removeStaleManagedEntries(destDir, expected, hashDB, func(entry fs.DirEntry) bool {
		return !entry.IsDir() && filepath.Ext(entry.Name()) == ext
	}, os.Remove)
	if err != nil {
		return err
	}
	for _, name := range result.preserved {
		fmt.Printf("  %s %s %s\n", ui.Warning.Render("!"), name, ui.Muted.Render("not managed by chai — skipping"))
	}
	return nil
}

func removeStaleManagedDirs(destDir string, expected map[string]bool, hashDB hash.DB, opts Options) (managedEntryChanges, error) {
	var result managedEntryChanges
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return result, fmt.Errorf("reading %s: %w", destDir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(destDir, entry.Name())
		if expected[path] {
			continue
		}
		stored, managed := hashDB[path]
		if !managed {
			result.preserved = append(result.preserved, entry.Name())
			continue
		}
		if !opts.Force {
			dirty, err := managedDirDirty(path, stored)
			if err != nil {
				return result, err
			}
			if dirty {
				if opts.Prompt == nil {
					return result, &DirtyError{Files: []string{path}}
				}
				remove, err := opts.Prompt(path)
				if err != nil {
					return result, err
				}
				if !remove {
					return result, fmt.Errorf("skill sync incomplete: modified stale destination %s was preserved", path)
				}
			}
		}
		if err := os.RemoveAll(path); err != nil {
			return result, fmt.Errorf("removing %s: %w", path, err)
		}
		delete(hashDB, path)
		result.removed = append(result.removed, entry.Name())
	}
	return result, nil
}

type managedEntryChanges struct {
	removed   []string
	preserved []string
}

func removeStaleManagedEntries(
	destDir string,
	expected map[string]bool,
	hashDB hash.DB,
	include func(fs.DirEntry) bool,
	remove func(string) error,
) (managedEntryChanges, error) {
	var result managedEntryChanges
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return result, fmt.Errorf("reading %s: %w", destDir, err)
	}
	for _, entry := range entries {
		if !include(entry) {
			continue
		}
		path := filepath.Join(destDir, entry.Name())
		if expected[path] {
			continue
		}
		if _, managed := hashDB[path]; managed {
			if err := remove(path); err != nil {
				return result, fmt.Errorf("removing %s: %w", path, err)
			}
			delete(hashDB, path)
			result.removed = append(result.removed, entry.Name())
		} else {
			result.preserved = append(result.preserved, entry.Name())
		}
	}
	return result, nil
}

func writeManagedFile(dest string, data []byte, hashDB hash.DB) error {
	if err := atomicWrite(dest, data); err != nil {
		return err
	}
	hashDB[dest] = hash.Sum(data)
	return nil
}
