package update

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/skill"
	chaisync "github.com/charliesbot/chai/internal/sync"
	"github.com/charliesbot/chai/internal/ui"
)

type Options struct {
	SyncOptions chaisync.Options
	CheckGit    func(context.Context) error
	Refresh     func(context.Context, string, githubskill.Identity, []string) (githubskill.Discovery, error)
	Sync        func(context.Context, *config.Config, string, chaisync.Options) error
}

func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	return RunWithHome(ctx, cfg, home, opts)
}

func RunWithHome(ctx context.Context, cfg *config.Config, home string, opts Options) error {
	var cleanupWarnings []error
	var refreshErr error
	if err := validateSourceNames(cfg, home); err != nil {
		return err
	}
	if len(cfg.Skills.GitHub) > 0 {
		checkGit := opts.CheckGit
		if checkGit == nil {
			checkGit = func(ctx context.Context) error {
				_, err := githubskill.CheckGit(ctx)
				return err
			}
		}
		if err := checkGit(ctx); err != nil {
			return err
		}
		refresh := githubRefreshFunc(opts.Refresh)
		if refresh == nil {
			refresh = githubskill.Refresh
		}
		warnings, err := runGitHubSkillsUpdate(ctx, cfg.Skills.GitHub, home, refresh)
		if err != nil {
			var missingErr *githubskill.MissingSkillsError
			if !errors.As(err, &missingErr) {
				return err
			}
			refreshErr = err
		}
		cleanupWarnings = append(cleanupWarnings, warnings...)
	}

	plugins := cfg.Antigravity.Plugins
	if !platform.HasPlatform(cfg.Platforms, "antigravity") {
		plugins = nil
	}
	if len(cfg.Deps) > 0 || len(plugins) > 0 {
		if err := updateDepsAndPlugins(cfg.Deps, plugins, home); err != nil {
			return errors.Join(err, refreshErr)
		}
	}
	if len(cfg.Skills.GitHub) == 0 {
		if len(cfg.Deps) == 0 && len(plugins) == 0 {
			fmt.Println(ui.Muted.Render("nothing to update"))
		}
		return nil
	}
	syncRun := opts.Sync
	if syncRun == nil {
		syncRun = chaisync.RunWithHome
	}
	if err := syncRun(ctx, cfg, home, opts.SyncOptions); err != nil {
		return errors.Join(append([]error{err, refreshErr}, cleanupWarnings...)...)
	}
	for _, warning := range cleanupWarnings {
		fmt.Printf("warning: previous cache cleanup is incomplete: %v\n", warning)
	}
	return refreshErr
}

func validateSourceNames(cfg *config.Config, home string) error {
	sources, err := skill.DiscoverLocal(cfg.Skills.Local, home, home)
	if err != nil {
		return err
	}
	for _, remote := range cfg.Skills.GitHub {
		for _, name := range remote.Include {
			sources = append(sources, skill.Source{Name: name, Path: remote.URL})
		}
	}
	return skill.ValidateUniqueNames(sources)
}
