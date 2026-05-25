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
	return removeStaleManagedEntries(destDir, expected, hashDB, func(entry fs.DirEntry) bool {
		return !entry.IsDir() && filepath.Ext(entry.Name()) == ext
	}, os.Remove)
}

func removeStaleManagedDirs(destDir string, expected map[string]bool, hashDB hash.DB) error {
	return removeStaleManagedEntries(destDir, expected, hashDB, func(entry fs.DirEntry) bool {
		return entry.IsDir()
	}, os.RemoveAll)
}

func removeStaleManagedEntries(
	destDir string,
	expected map[string]bool,
	hashDB hash.DB,
	include func(fs.DirEntry) bool,
	remove func(string) error,
) error {
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return fmt.Errorf("reading %s: %w", destDir, err)
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
			remove(path)
			delete(hashDB, path)
		} else {
			fmt.Printf("  %s %s %s\n", ui.Warning.Render("!"), entry.Name(), ui.Muted.Render("not managed by chai — skipping"))
		}
	}
	return nil
}

func writeManagedFile(dest string, data []byte, perm fs.FileMode, hashDB hash.DB) error {
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming %s → %s: %w", tmp, dest, err)
	}
	hashDB[dest] = hash.Sum(data)
	return nil
}
