package sync

import (
	"context"
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
	Force  bool
	DryRun bool
	Prompt PromptFunc
}

// Run executes the sync: copies instructions to all platform locations.
func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	return RunWithHome(ctx, cfg, home, opts)
}

// RunWithHome executes the sync using the given home directory.
func RunWithHome(ctx context.Context, cfg *config.Config, home string, opts Options) error {
	// Clone/pull deps before resolving paths (skip in dry-run)
	if !opts.DryRun {
		if err := deps.SyncWithHome(cfg.Deps, home); err != nil {
			return err
		}
	} else if len(cfg.Deps) > 0 {
		for name, url := range cfg.Deps {
			fmt.Printf("[dry-run] would clone/pull dep %s from %s\n", name, url)
		}
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
	for _, p := range platforms {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("sync interrupted: %w", err)
		}

		dest := filepath.Join(home, p.InstructionsPath)

		if !opts.Force {
			dirty, err := hashDB.IsDirty(dest)
			if err != nil {
				return err
			}
			if dirty {
				if opts.Prompt == nil {
					return &DirtyError{Files: []string{dest}}
				}
				overwrite, err := opts.Prompt(dest)
				if err != nil {
					return err
				}
				if !overwrite {
					fmt.Printf("skipped %s (%s)\n", p.Name, dest)
					continue
				}
			}
		}

		if opts.DryRun {
			fmt.Printf("[dry-run] would sync instructions → %s (%s)\n", p.Name, dest)
			continue
		}

		if err := atomicWrite(dest, content); err != nil {
			return fmt.Errorf("writing %s instructions to %s: %w", p.Name, dest, err)
		}
		hashDB[dest] = hash.Sum(content)
		fmt.Printf("synced instructions → %s (%s)\n", p.Name, dest)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("sync interrupted: %w", err)
	}

	if err := syncMCP(cfg, home, opts.DryRun); err != nil {
		return err
	}

	if opts.DryRun {
		return nil
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
