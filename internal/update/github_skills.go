package update

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/githubskill"
	"github.com/charliesbot/chai/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

type githubRefreshFunc func(context.Context, string, githubskill.Identity, []string) (githubskill.Discovery, error)

type githubSourceStatus int

const (
	githubSourceWaiting githubSourceStatus = iota
	githubSourceRefreshing
	githubSourceDone
	githubSourceFailed
)

type githubSourceItem struct {
	source    config.GitHubSkills
	label     string
	status    githubSourceStatus
	available []string
	duration  time.Duration
}

type githubSkillsModel struct {
	ctx      context.Context
	items    []githubSourceItem
	home     string
	refresh  githubRefreshFunc
	current  int
	frame    int
	done     bool
	err      error
	warnings []error
}

type githubSourceDoneMsg struct {
	index     int
	discovery githubskill.Discovery
	duration  time.Duration
	err       error
}

func newGitHubSkillsModel(ctx context.Context, sources []config.GitHubSkills, home string, refresh githubRefreshFunc) githubSkillsModel {
	items := make([]githubSourceItem, len(sources))
	for i, source := range sources {
		items[i] = githubSourceItem{
			source: source,
			label:  strings.TrimPrefix(source.URL, "https://github.com/"),
			status: githubSourceWaiting,
		}
	}
	if len(items) > 0 {
		items[0].status = githubSourceRefreshing
	}
	return githubSkillsModel{ctx: ctx, items: items, home: home, refresh: refresh}
}

func runGitHubSkillsUpdate(ctx context.Context, sources []config.GitHubSkills, home string, refresh githubRefreshFunc) ([]error, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	if refresh == nil {
		refresh = githubskill.Refresh
	}

	m := newGitHubSkillsModel(ctx, sources, home, refresh)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(nil))
	final, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("running GitHub skills update UI: %w", err)
	}
	result := final.(githubSkillsModel)
	return result.warnings, result.err
}

func (m githubSkillsModel) Init() tea.Cmd {
	if len(m.items) == 0 {
		return tea.Quit
	}
	return tea.Batch(m.startSource(0), m.tick())
}

func (m githubSkillsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	case githubSourceDoneMsg:
		item := &m.items[msg.index]
		item.duration = msg.duration
		if msg.err != nil {
			var cleanupErr *githubskill.CleanupError
			if !errors.As(msg.err, &cleanupErr) {
				item.status = githubSourceFailed
				m.err = fmt.Errorf("updating %s: %w", item.source.URL, msg.err)
				m.done = true
				return m, tea.Quit
			}
			m.warnings = append(m.warnings, fmt.Errorf("%s: %w", item.source.URL, cleanupErr))
		}
		item.status = githubSourceDone
		item.available = availableSkills(item.source, msg.discovery)

		next := msg.index + 1
		if next < len(m.items) {
			m.current = next
			m.items[next].status = githubSourceRefreshing
			return m, m.startSource(next)
		}

		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m githubSkillsModel) View() string {
	var b strings.Builder
	b.WriteString(ui.Title.Render("github skills"))
	b.WriteString("\n")
	if !m.done && len(m.items) > 0 {
		b.WriteString("\n  ")
		b.WriteString(ui.Muted.Render(fmt.Sprintf("refreshing %d/%d", m.current+1, len(m.items))))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	for _, item := range m.items {
		b.WriteString(m.renderSource(item))
	}
	return b.String()
}

func (m githubSkillsModel) renderSource(item githubSourceItem) string {
	icon := ui.Muted.Render("○")
	summary := ""
	switch item.status {
	case githubSourceRefreshing:
		icon = ui.Title.Render(spinnerFrames[m.frame%len(spinnerFrames)])
	case githubSourceDone:
		icon = ui.Check()
		summary = ui.Muted.Render(availabilitySummary(len(item.available)) + "  " + formatRefreshDuration(item.duration))
	case githubSourceFailed:
		icon = ui.Warning.Render("✗")
		summary = ui.Warning.Render("error")
	}

	line := fmt.Sprintf("  %s %s", icon, ui.Bold.Render(item.label))
	if summary != "" {
		line += "  " + summary
	}
	line += "\n"
	for _, name := range item.available {
		line += fmt.Sprintf("      %s %s\n", ui.Added.Render("+"), ui.ItemStyle.Render(name))
	}
	return line
}

func (m githubSkillsModel) startSource(index int) tea.Cmd {
	item := m.items[index]
	return func() tea.Msg {
		started := time.Now()
		id, err := githubskill.ParseCanonical(item.source.URL)
		if err != nil {
			return githubSourceDoneMsg{index: index, duration: time.Since(started), err: err}
		}
		discovery, err := m.refresh(m.ctx, m.home, id, item.source.Include)
		return githubSourceDoneMsg{index: index, discovery: discovery, duration: time.Since(started), err: err}
	}
}

func (m githubSkillsModel) tick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func availabilitySummary(count int) string {
	switch count {
	case 0:
		return "no new skills"
	case 1:
		return "1 new skill available"
	default:
		return fmt.Sprintf("%d new skills available", count)
	}
}

func formatRefreshDuration(duration time.Duration) string {
	return duration.Round(100 * time.Millisecond).String()
}
