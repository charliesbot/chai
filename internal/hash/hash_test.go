package hash

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSum(t *testing.T) {
	h := Sum([]byte("hello"))
	if h == "" {
		t.Fatal("Sum returned empty string")
	}
	// Same input = same hash
	if Sum([]byte("hello")) != h {
		t.Error("Sum not deterministic")
	}
	// Different input = different hash
	if Sum([]byte("world")) == h {
		t.Error("different inputs produced same hash")
	}
}

func TestDB_LoadSave(t *testing.T) {
	home := t.TempDir()

	// Load from nonexistent file returns empty DB
	db, err := Load(home)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(db) != 0 {
		t.Errorf("expected empty DB, got %d entries", len(db))
	}

	// Save and reload
	db["/some/path"] = "abc123"
	db["/other/path"] = "def456"
	if err := db.Save(home); err != nil {
		t.Fatalf("save error: %v", err)
	}

	loaded, err := Load(home)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if loaded["/some/path"] != "abc123" {
		t.Errorf("got %q, want %q", loaded["/some/path"], "abc123")
	}
	if loaded["/other/path"] != "def456" {
		t.Errorf("got %q, want %q", loaded["/other/path"], "def456")
	}
}

func TestDB_IsDirty_FirstSync(t *testing.T) {
	db := make(DB)

	dirty, err := db.IsDirty("/nonexistent/file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dirty {
		t.Error("first sync should not be dirty")
	}
}

func TestDB_IsDirty_Unchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	content := []byte("original content")
	os.WriteFile(path, content, 0644)

	db := DB{path: Sum(content)}

	dirty, err := db.IsDirty(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dirty {
		t.Error("unchanged file should not be dirty")
	}
}

func TestDB_IsDirty_Modified(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.md")
	os.WriteFile(path, []byte("original"), 0644)

	db := DB{path: Sum([]byte("original"))}

	// Modify the file
	os.WriteFile(path, []byte("modified"), 0644)

	dirty, err := db.IsDirty(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dirty {
		t.Error("modified file should be dirty")
	}
}

func TestDB_IsDirty_Deleted(t *testing.T) {
	db := DB{"/deleted/file": "somehash"}

	dirty, err := db.IsDirty("/deleted/file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dirty {
		t.Error("deleted file should not be dirty")
	}
}
