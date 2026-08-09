package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/resolve"
	"github.com/charliesbot/chai/internal/skill"
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

	platforms := platform.ForNames(cfg.Platforms)
	instructionPlatforms := platformsWithInstructions(platforms)
	if len(instructionPlatforms) > 0 && len(cfg.Instructions) == 0 {
		return fmt.Errorf("no instructions path set in config")
	}
	resolvedSkills, err := resolveConfiguredSkills(cfg, home)
	if err != nil {
		return err
	}

	hashDB, err := hash.Load(home)
	if err != nil {
		return err
	}
	status := newPlatformStatus(platforms)
	for _, p := range platforms {
		if p.InstructionsPath == "" {
			status.setNA(p.Name)
		}
	}

	var srcPath string
	var content []byte
	if len(instructionPlatforms) > 0 {
		var err error
		srcPath, err = resolve.PathWithHome(cfg.Instructions[0], home)
		if err != nil {
			return fmt.Errorf("resolving instructions path: %w", err)
		}

		content, err = os.ReadFile(srcPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("instructions file not found: %s", srcPath)
			}
			return fmt.Errorf("reading instructions: %w", err)
		}

		if opts.DryRun {
			fmt.Println(ui.Label.Render("instructions"))
			fmt.Printf("  %s %s\n", ui.Muted.Render("source:"), srcPath)
		}
	}

	// Group platforms by destination path so that platforms sharing an
	// instructions file only trigger one write and one dirty-detection prompt.
	destOrder := make([]string, 0, len(instructionPlatforms))
	destPlatforms := make(map[string][]platform.Platform, len(instructionPlatforms))
	for _, p := range instructionPlatforms {
		dest := filepath.Join(home, p.InstructionsPath)
		if _, ok := destPlatforms[dest]; !ok {
			destOrder = append(destOrder, dest)
		}
		destPlatforms[dest] = append(destPlatforms[dest], p)
	}
	instructionChanges := newItemChanges()
	skippedInstructionTargets := 0
	var incompleteErr error
	instructionName := filepath.Base(srcPath)
	contentHash := hash.Sum(content)

	for _, dest := range destOrder {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("sync interrupted: %w", err)
		}

		sharers := destPlatforms[dest]
		previousHash, managed := hashDB[dest]
		_, statErr := os.Stat(dest)
		existed := statErr == nil

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
					skippedInstructionTargets++
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
		hashDB[dest] = contentHash
		switch {
		case !managed:
			instructionChanges.record(instructionName, itemAdded)
		case !existed || previousHash != contentHash:
			instructionChanges.record(instructionName, itemUpdated)
		default:
			instructionChanges.record(instructionName, itemUnchanged)
		}
	}

	if !opts.DryRun && len(instructionPlatforms) > 0 {
		summary := instructionChanges.summary()
		if skippedInstructionTargets > 0 {
			skipped := fmt.Sprintf("%d %s skipped", skippedInstructionTargets, pluralize("target", skippedInstructionTargets))
			if summary == "" {
				summary = skipped
			} else {
				summary += " · " + skipped
			}
		}
		fmt.Println(ui.ResultLine("instructions", summary, status.statuses()))
		for _, detail := range instructionChanges.details() {
			fmt.Printf("   %s\n", detail.render())
		}
		if skippedInstructionTargets > 0 {
			incompleteErr = fmt.Errorf("instruction sync incomplete: %d modified targets were preserved", skippedInstructionTargets)
		}
	}

	if opts.DryRun {
		fmt.Println()
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("sync interrupted: %w", err)
	}

	if err := syncResolvedSkills(resolvedSkills, home, platforms, opts, hashDB); err != nil {
		return persistHashError(hashDB, home, opts.DryRun, err)
	}

	if err := syncAgents(cfg.Subagents.Paths, home, platforms, opts.DryRun, hashDB); err != nil {
		return persistHashError(hashDB, home, opts.DryRun, err)
	}

	if err := syncMCP(cfg, home, platforms, opts.DryRun); err != nil {
		return persistHashError(hashDB, home, opts.DryRun, err)
	}

	if platform.HasPlatform(cfg.Platforms, "droid") {
		if err := syncDroidCustomModels(cfg, home, opts.DryRun); err != nil {
			return persistHashError(hashDB, home, opts.DryRun, err)
		}
	}

	if opts.DryRun {
		return nil
	}

	if err := hashDB.Save(home); err != nil {
		return err
	}

	return incompleteErr
}

func persistHashError(hashDB hash.DB, home string, dryRun bool, operationErr error) error {
	if dryRun {
		return operationErr
	}
	return errors.Join(operationErr, hashDB.Save(home))
}

func resolveConfiguredSkills(cfg *config.Config, home string) ([]skill.Source, error) {
	resolved, err := resolveLocalSkillSources(cfg.Skills.Local, home, home)
	if err != nil {
		return nil, err
	}
	configuredSources := append([]skill.Source(nil), resolved...)
	for _, remote := range cfg.Skills.GitHub {
		for _, name := range remote.Include {
			configuredSources = append(configuredSources, skill.Source{Name: name, Path: remote.URL})
		}
	}
	if err := skill.ValidateUniqueNames(configuredSources); err != nil {
		return nil, err
	}
	var sourceErrors []error
	for _, remote := range cfg.Skills.GitHub {
		id, err := githubskill.ParseCanonical(remote.URL)
		if err != nil {
			return nil, err
		}
		cached, err := githubskill.ResolveCached(home, id, remote.Include)
		if err != nil {
			sourceErrors = append(sourceErrors, err)
			continue
		}
		for _, source := range cached {
			resolved = append(resolved, source)
		}
	}
	if len(sourceErrors) > 0 {
		return nil, errors.Join(sourceErrors...)
	}
	sort.Slice(resolved, func(i, j int) bool { return resolved[i].Name < resolved[j].Name })
	return resolved, nil
}

func ValidateSources(cfg *config.Config, home string) error {
	_, err := resolveConfiguredSkills(cfg, home)
	return err
}

// platformStatus tracks success/failure per platform, preserving order for UI rendering.
type platformStatus struct {
	order         []string
	state         map[string]ui.PlatformState
	displayByName map[string]string
}

func newPlatformStatus(platforms []platform.Platform) platformStatus {
	ps := platformStatus{
		order:         make([]string, 0, len(platforms)),
		state:         make(map[string]ui.PlatformState, len(platforms)),
		displayByName: make(map[string]string, len(platforms)),
	}
	for _, p := range platforms {
		display := platformDisplayName(p)
		ps.displayByName[p.Name] = display
		if _, ok := ps.state[display]; !ok {
			ps.order = append(ps.order, display)
			ps.state[display] = ui.PlatformOK
		}
	}
	return ps
}

func platformDisplayName(p platform.Platform) string {
	if p.Key == "antigravity" {
		return "Antigravity"
	}
	return p.Name
}

func platformsWithInstructions(platforms []platform.Platform) []platform.Platform {
	out := make([]platform.Platform, 0, len(platforms))
	for _, p := range platforms {
		if p.InstructionsPath != "" {
			out = append(out, p)
		}
	}
	return out
}

func (ps platformStatus) setFailed(name string) {
	display := ps.displayName(name)
	if _, ok := ps.state[display]; ok {
		ps.state[display] = ui.PlatformFailed
	}
}

func (ps platformStatus) setNA(name string) {
	display := ps.displayName(name)
	if _, ok := ps.state[display]; ok {
		ps.state[display] = ui.PlatformNA
	}
}

func (ps platformStatus) displayName(name string) string {
	if display, ok := ps.displayByName[name]; ok {
		return display
	}
	return name
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
