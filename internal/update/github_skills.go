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
	ctx       context.Context
	cancel    context.CancelFunc
	items     []githubSourceItem
	home      string
	refresh   githubRefreshFunc
	completed int
	next      int
	inFlight  int
	frame     int
	done      bool
	err       error
	warnings  []error
}

const maxConcurrentGitHubRefreshes = 3

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
	inFlight := min(maxConcurrentGitHubRefreshes, len(items))
	for i := 0; i < inFlight; i++ {
		items[i].status = githubSourceRefreshing
	}
	return githubSkillsModel{ctx: ctx, items: items, home: home, refresh: refresh, next: inFlight, inFlight: inFlight}
}

func runGitHubSkillsUpdate(ctx context.Context, sources []config.GitHubSkills, home string, refresh githubRefreshFunc) ([]error, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	if refresh == nil {
		refresh = githubskill.Refresh
	}

	refreshCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := newGitHubSkillsModel(refreshCtx, sources, home, refresh)
	m.cancel = cancel
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
	commands := []tea.Cmd{m.tick()}
	for i := 0; i < m.inFlight; i++ {
		commands = append(commands, m.startSource(i))
	}
	return tea.Batch(commands...)
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
		m.inFlight--
		m.completed++
		if msg.err != nil {
			var cleanupErr *githubskill.CleanupError
			if !errors.As(msg.err, &cleanupErr) {
				item.status = githubSourceFailed
				m.err = fmt.Errorf("updating %s: %w", item.source.URL, msg.err)
				m.done = true
				if m.cancel != nil {
					m.cancel()
				}
				return m, tea.Quit
			}
			m.warnings = append(m.warnings, fmt.Errorf("%s: %w", item.source.URL, cleanupErr))
		}
		item.status = githubSourceDone
		item.available = availableSkills(item.source, msg.discovery)

		if m.next < len(m.items) {
			next := m.next
			m.next++
			m.inFlight++
			m.items[next].status = githubSourceRefreshing
			return m, m.startSource(next)
		}
		if m.inFlight == 0 {
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m githubSkillsModel) View() string {
	var b strings.Builder
	b.WriteString(ui.Title.Render("github skills"))
	b.WriteString("\n")
	if !m.done && len(m.items) > 0 {
		b.WriteString("\n  ")
		b.WriteString(ui.Muted.Render(fmt.Sprintf("refreshing · %d/%d complete", m.completed, len(m.items))))
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
