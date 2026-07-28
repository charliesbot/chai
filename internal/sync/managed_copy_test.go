package sync

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/charliesbot/chai/internal/hash"
)

func TestRemoveStaleManagedEntries_PreservesCompletedChangesOnFailure(t *testing.T) {
	destDir := t.TempDir()
	removed := filepath.Join(destDir, "a-removed")
	failed := filepath.Join(destDir, "b-failed")
	for _, path := range []string{removed, failed} {
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatalf("creating stale directory: %v", err)
		}
	}
	hashDB := hash.DB{removed: "hash", failed: "hash"}
	removeErr := errors.New("remove failed")

	changes, err := removeStaleManagedEntries(
		destDir,
		map[string]bool{},
		hashDB,
		func(entry fs.DirEntry) bool { return entry.IsDir() },
		func(path string) error {
			if path == failed {
				return removeErr
			}
			return os.RemoveAll(path)
		},
	)

	if !errors.Is(err, removeErr) {
		t.Fatalf("error = %v, want %v", err, removeErr)
	}
	if len(changes.removed) != 1 || changes.removed[0] != "a-removed" {
		t.Fatalf("removed = %v, want [a-removed]", changes.removed)
	}
	if _, ok := hashDB[removed]; ok {
		t.Fatal("successfully removed path should leave the hash DB")
	}
	if _, ok := hashDB[failed]; !ok {
		t.Fatal("failed removal should remain managed in the hash DB")
	}
}
