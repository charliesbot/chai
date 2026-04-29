package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charliesbot/chai/internal/config"
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

	platforms := platform.ForNames(cfg.Platforms)
	status := newPlatformStatus(platforms)

	// Group platforms by destination path so that platforms sharing an
	// instructions file (e.g. Gemini + Antigravity both writing ~/.gemini/GEMINI.md)
	// only trigger one write and one dirty-detection prompt.
	destOrder := make([]string, 0, len(platforms))
	destPlatforms := make(map[string][]platform.Platform, len(platforms))
	for _, p := range platforms {
		dest := filepath.Join(home, p.InstructionsPath)
		if _, ok := destPlatforms[dest]; !ok {
			destOrder = append(destOrder, dest)
		}
		destPlatforms[dest] = append(destPlatforms[dest], p)
	}

	for _, dest := range destOrder {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("sync interrupted: %w", err)
		}

		sharers := destPlatforms[dest]

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
					for _, p := range sharers {
						status.setFailed(p.Name)
					}
					continue
				}
			}
		}

		if opts.DryRun {
			dryStatus := ui.Muted.Render("first sync")
			if _, ok := hashDB[dest]; ok {
				dirty, _ := hashDB.IsDirty(dest)
				if dirty {
					dryStatus = ui.Warning.Render("modified — will prompt")
				} else {
					dryStatus = ui.Muted.Render("unchanged")
				}
			}
			names := platformNames(sharers)
			fmt.Printf("  %s %s %s (%s)\n", ui.Arrow(), ui.Bold.Render(names), ui.Muted.Render(dest), dryStatus)
			continue
		}

		if err := atomicWrite(dest, content); err != nil {
			return fmt.Errorf("writing instructions to %s: %w", dest, err)
		}
		hashDB[dest] = hash.Sum(content)
	}

	if !opts.DryRun {
		instructionItems := make([]string, 0, len(destOrder)+1)
		instructionItems = append(instructionItems, srcPath)
		for _, dest := range destOrder {
			instructionItems = append(instructionItems, "→ "+dest)
		}
		fmt.Println(ui.Box("instructions", 0, status.statuses(), instructionItems))
	}

	if opts.DryRun {
		fmt.Println()
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("sync interrupted: %w", err)
	}

	if err := syncSkillsAndAgents(cfg.Skills.Paths, cfg.Subagents.Paths, home, platforms, opts.DryRun, hashDB); err != nil {
		return err
	}

	if err := syncMCP(cfg, home, platforms, opts.DryRun); err != nil {
		return err
	}

	if platform.HasPlatform(cfg.Platforms, "droid") {
		if err := syncDroidCustomModels(cfg, home, opts.DryRun); err != nil {
			return err
		}
	}

	if len(cfg.Gemini.Extensions) > 0 && !opts.DryRun && platform.HasPlatform(cfg.Platforms, "gemini") {
		names := make([]string, 0, len(cfg.Gemini.Extensions))
		for name := range cfg.Gemini.Extensions {
			names = append(names, name)
		}
		extStatus := make([]ui.PlatformStatus, len(platforms))
		for i, p := range platforms {
			if p.Name == "Gemini" {
				extStatus[i] = ui.PlatformStatus{Name: p.Name, State: ui.PlatformOK}
			} else {
				extStatus[i] = ui.PlatformStatus{Name: p.Name, State: ui.PlatformNA}
			}
		}
		fmt.Println(ui.Box("gemini extensions", len(names), extStatus, names))
	}

	if opts.DryRun {
		return nil
	}

	if err := hashDB.Save(home); err != nil {
		return err
	}

	return nil
}

// platformStatus tracks success/failure per platform, preserving order for UI rendering.
type platformStatus struct {
	order []string
	state map[string]ui.PlatformState
}

func newPlatformStatus(platforms []platform.Platform) platformStatus {
	ps := platformStatus{
		order: make([]string, 0, len(platforms)),
		state: make(map[string]ui.PlatformState, len(platforms)),
	}
	for _, p := range platforms {
		ps.order = append(ps.order, p.Name)
		ps.state[p.Name] = ui.PlatformOK
	}
	return ps
}

func (ps platformStatus) setFailed(name string) {
	if _, ok := ps.state[name]; ok {
		ps.state[name] = ui.PlatformFailed
	}
}

func (ps platformStatus) setNA(name string) {
	if _, ok := ps.state[name]; ok {
		ps.state[name] = ui.PlatformNA
	}
}

func (ps platformStatus) statuses() []ui.PlatformStatus {
	out := make([]ui.PlatformStatus, len(ps.order))
	for i, name := range ps.order {
		out[i] = ui.PlatformStatus{Name: name, State: ps.state[name]}
	}
	return out
}

// platformNames joins the names of the given platforms with " + " for display.
func platformNames(platforms []platform.Platform) string {
	names := make([]string, len(platforms))
	for i, p := range platforms {
		names[i] = p.Name
	}
	return strings.Join(names, " + ")
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
