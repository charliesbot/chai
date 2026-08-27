package add

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	"github.com/charliesbot/chai/internal/skill"
	chaisync "github.com/charliesbot/chai/internal/sync"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type Request struct {
	Source string
	Skills []string
	List   bool
}

type Options struct {
	Acquire     githubskill.AcquireFunc
	Sync        func(context.Context, *config.Config, string, chaisync.Options) error
	Progress    func(string, func() error) error
	SyncOptions chaisync.Options
	Output      io.Writer
}

type progressDone struct{ err error }

type progressModel struct {
	spinner spinner.Model
	label   string
	run     func() error
	err     error
}

func (m progressModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, func() tea.Msg { return progressDone{err: m.run()} })
}

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progressDone:
		m.err = msg.err
		return m, tea.Quit
	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

func (m progressModel) View() string {
	return fmt.Sprintf("\n %s %s", m.spinner.View(), m.label)
}

func ParseArgs(args []string) (Request, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return Request{}, fmt.Errorf("add requires a source as the first argument")
	}
	request := Request{Source: args[0]}
	seenSkill := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--list":
			request.List = true
		case "--yes", "-y":
			// Accepted for compatibility; add executes immediately.
		case "--global", "-g":
			// Accepted for compatibility; chai currently manages only global config.
		case "--skill":
			if seenSkill {
				return Request{}, fmt.Errorf("--skill may be specified only once")
			}
			seenSkill = true
			start := i + 1
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				request.Skills = append(request.Skills, args[i])
			}
			if i+1 == start {
				return Request{}, fmt.Errorf("--skill requires at least one skill name")
			}
		default:
			return Request{}, fmt.Errorf("unknown add option or unexpected argument %q", args[i])
		}
	}
	for _, name := range request.Skills {
		if !skill.ValidName(name) {
			return Request{}, fmt.Errorf("invalid skill name %q", name)
		}
	}
	return request, nil
}

func Run(ctx context.Context, cfg *config.Config, args []string, opts Options) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return RunWithHome(ctx, cfg, filepath.Join(home, "chai.toml"), home, args, opts)
}

func RunWithHome(ctx context.Context, cfg *config.Config, configPath, home string, args []string, opts Options) error {
	request, err := ParseArgs(args)
	if err != nil {
		return err
	}
	if request.List && len(request.Skills) > 0 {
		return fmt.Errorf("--list cannot be combined with --skill")
	}
	if isLocalInput(request.Source) {
		return addLocal(ctx, cfg, configPath, home, request, opts)
	}
	return addRemote(ctx, cfg, configPath, home, request, opts)
}

func isLocalInput(source string) bool {
	return source == "~" || strings.HasPrefix(source, "~/") || filepath.IsAbs(source) ||
		strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../")
}

func addLocal(ctx context.Context, cfg *config.Config, configPath, home string, request Request, opts Options) error {
	if request.List {
		return fmt.Errorf("--list is supported only for GitHub sources")
	}
	if len(request.Skills) > 0 {
		return fmt.Errorf("--skill is supported only for GitHub sources")
	}
	normalized, err := config.NormalizeLocalSkillPath(request.Source, home)
	if err != nil {
		return err
	}
	if err := config.AddLocalSkillSourceAtomic(configPath, cfg, normalized, home, func(candidate *config.Config) error {
		discovered, err := skill.DiscoverLocal(candidate.Skills.Local, filepath.Dir(configPath), home)
		if err != nil {
			return err
		}
		if err := rejectNameConflicts(discovered, nil, candidate, ""); err != nil {
			return err
		}
		return chaisync.ValidateSources(candidate, home)
	}); err != nil {
		return err
	}
	return runSync(ctx, cfg, home, opts)
}

func addRemote(ctx context.Context, cfg *config.Config, configPath, home string, request Request, opts Options) error {
	id, err := githubskill.ParseInput(request.Source)
	if err != nil {
		return err
	}
	acquire := opts.Acquire
	if acquire == nil {
		acquire = githubskill.Acquire
	}

	var selected []string
	_, acquireErr := acquire(ctx, home, id, func(discovery githubskill.Discovery) (githubskill.AcquisitionDecision, error) {
		if request.List {
			printDiscovery(output(opts), discovery)
			return githubskill.AcquisitionDecision{}, nil
		}

		selected = append([]string(nil), request.Skills...)
		if len(selected) == 0 {
			if len(discovery.Problems) > 0 {
				return githubskill.AcquisitionDecision{}, fmt.Errorf("cannot add all skills because %s", discovery.Problems[0])
			}
			for _, candidate := range discovery.Candidates {
				selected = append(selected, candidate.Name)
			}
			if len(selected) == 0 {
				return githubskill.AcquisitionDecision{}, fmt.Errorf("GitHub source contains no valid skills")
			}
		}

		for _, source := range cfg.Skills.GitHub {
			if source.URL == id.URL() {
				if len(request.Skills) > 0 {
					selected = append(selected, source.Include...)
				}
				break
			}
		}
		selected = sortedUnique(selected)

		return githubskill.AcquisitionDecision{
			Names: selected,
			Install: func() error {
				return config.ReconcileGitHubSkillSourceAtomic(
					configPath,
					cfg,
					config.GitHubSkills{URL: id.URL(), Include: selected},
					func(candidate *config.Config) error {
						locals, err := skill.DiscoverLocal(candidate.Skills.Local, filepath.Dir(configPath), home)
						if err != nil {
							return err
						}
						selectedSources := make([]skill.Source, len(selected))
						for i, name := range selected {
							selectedSources[i] = skill.Source{Name: name, Path: id.URL()}
						}
						if err := rejectNameConflicts(selectedSources, locals, candidate, id.URL()); err != nil {
							return err
						}
						if err := chaisync.ValidateUnmanagedSkillDestinations(selected, home, candidate.Platforms); err != nil {
							return err
						}
						return chaisync.ValidateSources(candidate, home)
					},
				)
			},
		}, nil
	}, func(phase githubskill.AcquisitionPhase, count int, operation func() error) error {
		label := "Inspecting " + request.Source
		if phase == githubskill.AcquisitionFetching {
			label = fmt.Sprintf("Fetching %d selected skill(s)", count)
		}
		return withProgress(opts, label, operation)
	})
	if request.List {
		return acquireErr
	}
	var cleanupErr *githubskill.CleanupError
	if acquireErr != nil && !errors.As(acquireErr, &cleanupErr) {
		return acquireErr
	}
	syncErr := runSync(ctx, cfg, home, opts)
	if cleanupErr != nil {
		incompleteCleanup := fmt.Errorf("previous cache cleanup is incomplete: %w", cleanupErr)
		if syncErr != nil {
			return errors.Join(syncErr, incompleteCleanup)
		}
		return fmt.Errorf("source was recorded and synced, but %w", incompleteCleanup)
	}
	return syncErr
}

func rejectNameConflicts(selected, locals []skill.Source, cfg *config.Config, skipURL string) error {
	sources := append([]skill.Source(nil), locals...)
	for _, remote := range cfg.Skills.GitHub {
		if remote.URL == skipURL {
			continue
		}
		for _, name := range remote.Include {
			sources = append(sources, skill.Source{Name: name, Path: remote.URL})
		}
	}
	sources = append(sources, selected...)
	return skill.ValidateUniqueNames(sources)
}

func withProgress(opts Options, label string, operation func() error) error {
	if opts.Progress != nil {
		return opts.Progress(label, operation)
	}
	writer := output(opts)
	if opts.Output != nil || !stdoutIsTerminal() {
		fmt.Fprintf(writer, "%s…\n", label)
		return operation()
	}
	model := progressModel{spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot)), label: label, run: operation}
	final, err := tea.NewProgram(model, tea.WithInput(nil), tea.WithOutput(writer)).Run()
	if err != nil {
		return err
	}
	return final.(progressModel).err
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func printDiscovery(writer io.Writer, discovery githubskill.Discovery) {
	for _, candidate := range discovery.Candidates {
		if candidate.Description == "" {
			fmt.Fprintln(writer, candidate.Name)
		} else {
			fmt.Fprintf(writer, "%s\t%s\n", candidate.Name, candidate.Description)
		}
	}
	for _, problem := range discovery.Problems {
		fmt.Fprintf(writer, "invalid\t%s\n", problem)
	}
}

func sortedUnique(values []string) []string {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	values = values[:0]
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func runSync(ctx context.Context, cfg *config.Config, home string, opts Options) error {
	run := opts.Sync
	if run == nil {
		run = chaisync.RunWithHome
	}
	if err := run(ctx, cfg, home, opts.SyncOptions); err != nil {
		return fmt.Errorf("source was recorded but sync is incomplete: %w", err)
	}
	return nil
}

func output(opts Options) io.Writer {
	if opts.Output != nil {
		return opts.Output
	}
	return os.Stdout
}
