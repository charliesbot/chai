package sync

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/resolve"
)

// Run executes the sync: copies instructions to all platform locations.
func Run(cfg *config.Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	return RunWithHome(cfg, home)
}

// RunWithHome executes the sync using the given home directory.
func RunWithHome(cfg *config.Config, home string) error {
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

	platforms := platform.All()
	for _, p := range platforms {
		dest := filepath.Join(home, p.InstructionsPath)
		if err := atomicWrite(dest, content); err != nil {
			return fmt.Errorf("writing %s instructions to %s: %w", p.Name, dest, err)
		}
		fmt.Printf("synced instructions → %s (%s)\n", p.Name, dest)
	}

	if err := syncMCP(cfg, home); err != nil {
		return err
	}

	return nil
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
