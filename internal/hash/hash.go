package hash

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const dbFile = ".chai/hashes.json"

// DB is a map of file paths to their MD5 hashes from the last sync.
type DB map[string]string

// Load reads the hash DB from ~/.chai/hashes.json.
// Returns an empty DB if the file doesn't exist.
func Load(home string) (DB, error) {
	path := filepath.Join(home, dbFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(DB), nil
		}
		return nil, fmt.Errorf("reading hash DB: %w", err)
	}

	var db DB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("parsing hash DB: %w", err)
	}
	return db, nil
}

// Save writes the hash DB to ~/.chai/hashes.json.
func (db DB) Save(home string) error {
	path := filepath.Join(home, dbFile)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating hash DB directory: %w", err)
	}

	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling hash DB: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing hash DB: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming hash DB: %w", err)
	}
	return nil
}

// Sum returns the MD5 hex digest of data.
func Sum(data []byte) string {
	return fmt.Sprintf("%x", md5.Sum(data))
}

// IsDirty checks whether the file at path has been modified since the last sync.
// Returns true if the file exists and its hash doesn't match the stored hash.
// Returns false if the file doesn't exist (first sync) or hashes match.
func (db DB) IsDirty(path string) (bool, error) {
	stored, ok := db[path]
	if !ok {
		// No stored hash = first sync for this file
		return false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File was deleted — not dirty, just gone
			return false, nil
		}
		return false, fmt.Errorf("reading %s for dirty check: %w", path, err)
	}

	current := Sum(data)
	return current != stored, nil
}
