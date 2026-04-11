package sync

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/deps"
	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/resolve"
)

// Options controls sync behavior.
type Options struct {
	Force bool
}

// Run executes the sync: copies instructions to all platform locations.
func Run(cfg *config.Config, opts Options) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	return RunWithHome(cfg, home, opts)
}

// RunWithHome executes the sync using the given home directory.
func RunWithHome(cfg *config.Config, home string, opts Options) error {
	// Clone/pull deps before resolving paths
	if err := deps.SyncWithHome(cfg.Deps, home); err != nil {
		return err
	}

	if cfg.Instructions == "" {
		return fmt.Errorf("no instructions path set in config")
	}

	srcPath, err := resolve.PathWithHome(cfg.Instructions, home)
	if err != nil {
		return fmt.Errorf("resolving instructions path: %w", err)
	}

	content, err := os.ReadFile(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("instructions file not found: %s", srcPath)
		}
		return fmt.Errorf("reading instructions: %w", err)
	}

	hashDB, err := hash.Load(home)
	if err != nil {
		return err
	}

	platforms := platform.All()
	var dirtyFiles []string

	if !opts.Force {
		for _, p := range platforms {
			dest := filepath.Join(home, p.InstructionsPath)
			dirty, err := hashDB.IsDirty(dest)
			if err != nil {
				return err
			}
			if dirty {
				dirtyFiles = append(dirtyFiles, dest)
			}
		}
	}

	if len(dirtyFiles) > 0 {
		return &DirtyError{Files: dirtyFiles}
	}

	for _, p := range platforms {
		dest := filepath.Join(home, p.InstructionsPath)
		if err := atomicWrite(dest, content); err != nil {
			return fmt.Errorf("writing %s instructions to %s: %w", p.Name, dest, err)
		}
		hashDB[dest] = hash.Sum(content)
		fmt.Printf("synced instructions → %s (%s)\n", p.Name, dest)
	}

	if err := syncMCP(cfg, home); err != nil {
		return err
	}

	if err := hashDB.Save(home); err != nil {
		return err
	}

	return nil
}

// DirtyError is returned when target files have been manually edited since the last sync.
type DirtyError struct {
	Files []string
}

func (e *DirtyError) Error() string {
	msg := "the following files were modified since last sync:\n"
	for _, f := range e.Files {
		msg += fmt.Sprintf("  - %s\n", f)
	}
	msg += "run with --force to overwrite"
	return msg
}

// atomicWrite writes data to a temp file then renames it to the target path.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}
