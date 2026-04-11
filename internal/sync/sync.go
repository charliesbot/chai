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
	"github.com/charliesbot/chai/internal/ui"
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
	if opts.DryRun {
		fmt.Println(ui.DryRunTag() + " " + ui.Muted.Render("previewing sync — no files will be written"))
		fmt.Println()
	}

	// Clone/pull deps before resolving paths (skip in dry-run)
	if !opts.DryRun {
		if err := deps.SyncWithHome(cfg.Deps, home); err != nil {
			return err
		}
	} else if len(cfg.Deps) > 0 {
		fmt.Println(ui.Label.Render("deps"))
		for name, url := range cfg.Deps {
			fmt.Printf("  %s %s %s\n", ui.Arrow(), ui.Bold.Render(name), ui.Muted.Render(url))
		}
		fmt.Println()
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

	if opts.DryRun {
		fmt.Println(ui.Label.Render("instructions"))
		fmt.Printf("  %s %s\n", ui.Muted.Render("source:"), srcPath)
	}

	platforms := platform.All()
	for _, p := range platforms {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("sync interrupted: %w", err)
		}

		dest := filepath.Join(home, p.InstructionsPath)

		if !opts.Force && !opts.DryRun {
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
					fmt.Println(ui.SkippedLine(p.Name, dest))
					continue
				}
			}
		}

		if opts.DryRun {
			status := ui.Muted.Render("first sync")
			if _, ok := hashDB[dest]; ok {
				dirty, _ := hashDB.IsDirty(dest)
				if dirty {
					status = ui.Warning.Render("modified — will prompt")
				} else {
					status = ui.Muted.Render("unchanged")
				}
			}
			fmt.Printf("  %s %s %s (%s)\n", ui.Arrow(), ui.Bold.Render(p.Name), ui.Muted.Render(dest), status)
			continue
		}

		if err := atomicWrite(dest, content); err != nil {
			return fmt.Errorf("writing %s instructions to %s: %w", p.Name, dest, err)
		}
		hashDB[dest] = hash.Sum(content)
		fmt.Println(ui.SyncedLine(p.Name, dest))
	}

	if opts.DryRun {
		fmt.Println()
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("sync interrupted: %w", err)
	}

	if err := syncSkills(cfg.Skills.Paths, home, opts.DryRun); err != nil {
		return err
	}

	if err := syncAgents(cfg.Agents.Paths, home, opts.DryRun); err != nil {
		return err
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
