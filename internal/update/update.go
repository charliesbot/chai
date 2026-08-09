package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/deps"
	"github.com/charliesbot/chai/internal/githubskill"
	"github.com/charliesbot/chai/internal/platform"
	"github.com/charliesbot/chai/internal/skill"
	chaisync "github.com/charliesbot/chai/internal/sync"
	"github.com/charliesbot/chai/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

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
	if len(cfg.Skills.GitHub) > 0 {
		if err := validateSourceNames(cfg, home); err != nil {
			return err
		}
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
		refresh := opts.Refresh
		if refresh == nil {
			refresh = githubskill.Refresh
		}
		for _, source := range cfg.Skills.GitHub {
			id, err := githubskill.ParseCanonical(source.URL)
			if err != nil {
				return err
			}
			discovery, err := refresh(ctx, home, id, source.Include)
			if err != nil {
				var cleanupErr *githubskill.CleanupError
				if !errors.As(err, &cleanupErr) {
					return fmt.Errorf("updating %s: %w", source.URL, err)
				}
				cleanupWarnings = append(cleanupWarnings, fmt.Errorf("%s: %w", source.URL, cleanupErr))
			}
			reportAvailableSkills(source, discovery)
		}
	}

	plugins := cfg.Antigravity.Plugins
	if !platform.HasPlatform(cfg.Platforms, "antigravity") {
		plugins = nil
	}
	if len(cfg.Deps) > 0 || len(plugins) > 0 {
		if err := runLegacyWithHome(cfg.Deps, plugins, home); err != nil {
			return err
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
		return errors.Join(append([]error{err}, cleanupWarnings...)...)
	}
	for _, warning := range cleanupWarnings {
		fmt.Printf("warning: previous cache cleanup is incomplete: %v\n", warning)
	}
	return nil
}

func validateSourceNames(cfg *config.Config, home string) error {
	locals, err := skill.DiscoverLocal(cfg.Skills.Local, home, home)
	if err != nil {
		return err
	}
	localNames := make(map[string]string, len(locals))
	for _, source := range locals {
		localNames[source.Name] = source.Path
	}
	for _, remote := range cfg.Skills.GitHub {
		for _, name := range remote.Include {
			if localPath, exists := localNames[name]; exists {
				return fmt.Errorf("duplicate skill name %q from %s and %s", name, localPath, remote.URL)
			}
		}
	}
	return nil
}

func reportAvailableSkills(source config.GitHubSkills, discovery githubskill.Discovery) {
	available := availableSkills(source, discovery)
	if len(available) > 0 {
		fmt.Printf("%s: new skills available: %s\n", source.URL, strings.Join(available, ", "))
	}
}

func availableSkills(source config.GitHubSkills, discovery githubskill.Discovery) []string {
	selected := make(map[string]bool, len(source.Include))
	for _, name := range source.Include {
		selected[name] = true
	}
	counts := make(map[string]int)
	for _, candidate := range discovery.Candidates {
		counts[candidate.Name]++
	}
	var available []string
	for name, count := range counts {
		if count == 1 && !selected[name] {
			available = append(available, name)
		}
	}
	sort.Strings(available)
	return available
}

// runLegacyWithHome updates dependency repositories and Antigravity plugins.
func runLegacyWithHome(depMap map[string]config.Dep, plugins map[string]string, home string) error {
	if len(depMap) == 0 && len(plugins) == 0 {
		return nil
	}

	m := newModel(depMap, plugins, home)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return fmt.Errorf("running update UI: %w", err)
	}

	result := final.(model)
	if result.err != nil {
		return result.err
	}

	return nil
}

// itemKind distinguishes deps from Antigravity plugins.
type itemKind int

const (
	kindDep itemKind = iota
	kindPlugin
)

// item tracks the state of a single dep or plugin during update.
type item struct {
	name   string
	url    string
	kind   itemKind
	dep    config.Dep // only meaningful when kind == kindDep
	status string     // "waiting", "updating", "done", "error"
	action string     // result description
}

type model struct {
	items   []item
	home    string
	current int
	frame   int
	done    bool
	err     error
}

type tickMsg struct{}
type itemDoneMsg struct {
	index  int
	action string
	err    error
}

func newModel(depMap map[string]config.Dep, plugins map[string]string, home string) model {
	var items []item

	for _, name := range sortedDepKeys(depMap) {
		items = append(items, item{
			name:   name,
			url:    depMap[name].URL,
			kind:   kindDep,
			dep:    depMap[name],
			status: "waiting",
		})
	}

	for _, name := range sortedStringKeys(plugins) {
		items = append(items, item{
			name:   name,
			url:    plugins[name],
			kind:   kindPlugin,
			status: "waiting",
		})
	}

	if len(items) > 0 {
		items[0].status = "updating"
	}

	return model{items: items, home: home, current: 0}
}

func (m model) Init() tea.Cmd {
	if len(m.items) == 0 {
		return tea.Quit
	}
	return tea.Batch(
		m.startItem(0),
		m.tick(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.done = true
			return m, tea.Quit
		}

	case tickMsg:
		m.frame++
		if !m.done {
			return m, m.tick()
		}

	case itemDoneMsg:
		if msg.err != nil {
			m.items[msg.index].status = "error"
			m.items[msg.index].action = "error"
			m.err = fmt.Errorf("%s: %w", m.items[msg.index].name, msg.err)
			m.done = true
			return m, tea.Quit
		}
		m.items[msg.index].status = "done"
		m.items[msg.index].action = msg.action

		// Start next item
		next := msg.index + 1
		if next < len(m.items) {
			m.current = next
			m.items[next].status = "updating"
			return m, m.startItem(next)
		}

		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

func (m model) View() string {
	hasDeps, hasPlugins := false, false
	for _, it := range m.items {
		switch it.kind {
		case kindDep:
			hasDeps = true
		case kindPlugin:
			hasPlugins = true
		}
	}

	s := ""
	if hasDeps {
		s += ui.Title.Render("deps") + "\n\n"
		for _, it := range m.items {
			if it.kind == kindDep {
				s += m.renderItem(it)
			}
		}
		s += "\n"
	}
	if hasPlugins {
		s += ui.Title.Render("antigravity plugins") + "\n\n"
		for _, it := range m.items {
			if it.kind == kindPlugin {
				s += m.renderItem(it)
			}
		}
		s += "\n"
	}
	return s
}

func (m model) renderItem(it item) string {
	icon := m.statusIcon(it.status)
	name := ui.Bold.Render(it.name)
	url := ui.Muted.Render(it.url)

	switch it.status {
	case "done":
		action := statusStyle(it.action)
		return fmt.Sprintf("  %s %s  %s\n    %s\n", icon, name, action, url)
	case "error":
		return fmt.Sprintf("  %s %s  %s\n    %s\n", icon, name, ui.Warning.Render("error"), url)
	default:
		return fmt.Sprintf("  %s %s\n    %s\n", icon, name, url)
	}
}

var (
	statusGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("120")) // pastel green
	statusYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("222")) // pastel yellow
	statusPink   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")) // pastel pink
)

func statusStyle(action string) string {
	switch action {
	case "cloned", "installed":
		return statusGreen.Render(action)
	case "pulled":
		return statusYellow.Render(action)
	case "up to date":
		return statusPink.Render(action)
	default:
		// "cloned + built" etc
		return statusGreen.Render(action)
	}
}

func (m model) statusIcon(status string) string {
	switch status {
	case "done":
		return ui.Check()
	case "error":
		return ui.Warning.Render("✗")
	case "updating":
		frame := m.frame % len(spinnerFrames)
		return lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(spinnerFrames[frame])
	default:
		return ui.Muted.Render("○")
	}
}

func (m model) startItem(index int) tea.Cmd {
	it := m.items[index]
	home := m.home
	return func() tea.Msg {
		switch it.kind {
		case kindDep:
			result := deps.SyncOne(it.name, it.dep, home)
			if result.Err != nil {
				return itemDoneMsg{index: index, err: result.Err}
			}
			action := string(result.Action)
			if result.Built {
				action += " + built"
			}
			return itemDoneMsg{index: index, action: action}

		case kindPlugin:
			cmd := exec.Command("agy", "plugin", "install", it.url)
			out, err := cmd.CombinedOutput()
			if err != nil {
				// "already installed" is not a real error.
				if strings.Contains(string(out), "already installed") {
					return itemDoneMsg{index: index, action: "up to date"}
				}
				return itemDoneMsg{index: index, err: fmt.Errorf("%s", string(out))}
			}
			return itemDoneMsg{index: index, action: "installed"}
		}

		return itemDoneMsg{index: index, err: fmt.Errorf("unknown item kind")}
	}
}

func (m model) tick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(_ time.Time) tea.Msg {
		return tickMsg{}
	})
}

func sortedDepKeys(m map[string]config.Dep) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
