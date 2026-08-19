package sync

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/charliesbot/chai/internal/hash"
)

type failingDirectoryAdapter struct {
	directoryDestinationAdapter
	failPath string
	err      error
}

func (adapter failingDirectoryAdapter) remove(path string) error {
	if path == adapter.failPath {
		return adapter.err
	}
	return os.RemoveAll(path)
}

func TestReconcileManagedDestinationsPreservesCompletedChangesOnFailure(t *testing.T) {
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

	result, err := reconcileManagedDestinations(
		destDir,
		nil,
		failingDirectoryAdapter{failPath: failed, err: removeErr},
		hashDB,
		reconciliationPolicy{RemoveStale: true},
	)

	if !errors.Is(err, removeErr) {
		t.Fatalf("error = %v, want %v", err, removeErr)
	}
	if change := result.changes.items["a-removed"]; change != itemRemoved {
		t.Fatalf("a-removed change = %v, want removed", change)
	}
	if _, ok := hashDB[removed]; ok {
		t.Fatal("successfully removed path should leave the hash DB")
	}
	if _, ok := hashDB[failed]; !ok {
		t.Fatal("failed removal should remain managed in the hash DB")
	}
}

func TestReconcileManagedFilesReturnsChangesAndPreservesUnmanagedEntries(t *testing.T) {
	destDir := t.TempDir()
	managed := filepath.Join(destDir, "managed.md")
	unmanaged := filepath.Join(destDir, "notes.md")
	if err := os.WriteFile(managed, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unmanaged, []byte("notes"), 0644); err != nil {
		t.Fatal(err)
	}
	hashDB := hash.DB{managed: hash.Sum([]byte("old"))}

	result, err := reconcileManagedFiles(
		destDir,
		".md",
		[]managedDesired{{Name: "current.md", Content: []byte("new")}},
		hashDB,
		generatedFileReconciliationPolicy(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.changes.items["managed.md"] != itemRemoved || result.changes.items["current.md"] != itemAdded {
		t.Fatalf("changes = %#v", result.changes.items)
	}
	if len(result.preserved) != 1 || result.preserved[0] != "notes.md" {
		t.Fatalf("preserved = %v", result.preserved)
	}
	if _, err := os.Stat(unmanaged); err != nil {
		t.Fatalf("unmanaged file was not preserved: %v", err)
	}
}

func TestReconcileManagedDestinationCanPreserveDeclinedWrite(t *testing.T) {
	destDir := t.TempDir()
	destination := filepath.Join(destDir, "AGENTS.md")
	if err := os.WriteFile(destination, []byte("edited"), 0644); err != nil {
		t.Fatal(err)
	}
	hashDB := hash.DB{destination: hash.Sum([]byte("original"))}

	result, err := reconcileManagedFiles(
		destDir,
		".md",
		[]managedDesired{{Name: "AGENTS.md", Content: []byte("replacement")}},
		hashDB,
		reconciliationPolicy{
			ProtectDirty:     true,
			PreserveDeclined: true,
			Prompt:           func(string) (bool, error) { return false, nil },
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.skipped) != 1 || result.skipped[0] != destination {
		t.Fatalf("skipped = %v", result.skipped)
	}
	if data, err := os.ReadFile(destination); err != nil || string(data) != "edited" {
		t.Fatalf("destination = %q, err=%v", data, err)
	}
}
