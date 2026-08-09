package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	"github.com/charliesbot/chai/internal/hash"
	"github.com/charliesbot/chai/internal/platform"
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
	resolvedSkills, err := resolveConfiguredSkills(cfg, home)
	if err != nil {
		return err
	}

	hashDB, err := hash.Load(home)
	if err != nil {
		return err
	}
	incompleteErr, err := syncInstructions(ctx, cfg, home, platforms, opts, hashDB)
	if err != nil {
		return err
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
