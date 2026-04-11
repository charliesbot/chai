package update

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/charliesbot/chai/internal/deps"
	"github.com/charliesbot/chai/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Run executes the update with a Bubbletea TUI.
func Run(depMap map[string]string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	return RunWithHome(depMap, home)
}

// RunWithHome executes the update with a Bubbletea TUI using the given home directory.
func RunWithHome(depMap map[string]string, home string) error {
	if len(depMap) == 0 {
		fmt.Println(ui.Muted.Render("no deps configured"))
		return nil
	}

	m := newModel(depMap, home)
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

// dep tracks the state of a single dependency during update.
type dep struct {
	name   string
	url    string
	status string // "waiting", "updating", "done", "error"
	result *deps.Result
}

type model struct {
	deps    []dep
	home    string
	current int
	frame   int
	done    bool
	err     error
}

type tickMsg struct{}
type depDoneMsg struct {
	index  int
	result deps.Result
}

func newModel(depMap map[string]string, home string) model {
	// Sort dep names for deterministic order
	names := make([]string, 0, len(depMap))
	for name := range depMap {
		names = append(names, name)
	}
	sort.Strings(names)

	d := make([]dep, len(names))
	for i, name := range names {
		status := "waiting"
		if i == 0 {
			status = "updating"
		}
		d[i] = dep{name: name, url: depMap[name], status: status}
	}

	return model{deps: d, home: home, current: 0}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.startUpdate(0),
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

	case depDoneMsg:
		m.deps[msg.index].result = &msg.result
		if msg.result.Err != nil {
			m.deps[msg.index].status = "error"
			m.err = fmt.Errorf("%s: %w", msg.result.Name, msg.result.Err)
			m.done = true
			return m, tea.Quit
		}
		m.deps[msg.index].status = "done"

		// Start next dep
		next := msg.index + 1
		if next < len(m.deps) {
			m.current = next
			m.deps[next].status = "updating"
			return m, m.startUpdate(next)
		}

		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

func (m model) View() string {
	s := ui.Title.Render("updating deps") + "\n\n"

	for _, d := range m.deps {
		icon := m.statusIcon(d.status)
		name := ui.Bold.Render(d.name)
		url := ui.Muted.Render(d.url)

		switch d.status {
		case "done":
			action := actionStyle(d.result.Action)
			s += fmt.Sprintf("  %s %s %s %s\n", icon, name, url, action)
		case "error":
			s += fmt.Sprintf("  %s %s %s %s\n", icon, name, url, ui.Warning.Render("error"))
		default:
			s += fmt.Sprintf("  %s %s %s\n", icon, name, url)
		}
	}

	if m.done {
		s += "\n"
	}

	return s
}

func (m model) statusIcon(status string) string {
	switch status {
	case "done":
		return ui.Check()
	case "error":
		return ui.Warning.Render("✗")
	case "updating":
		frame := m.frame % len(spinner)
		return lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render(spinner[frame])
	default: // waiting
		return ui.Muted.Render("○")
	}
}

func actionStyle(action deps.Action) string {
	switch action {
	case deps.ActionCloned:
		return ui.Success.Render("cloned")
	case deps.ActionPulled:
		return ui.Success.Render("pulled")
	case deps.ActionCurrent:
		return ui.Muted.Render("up to date")
	default:
		return ""
	}
}

func (m model) startUpdate(index int) tea.Cmd {
	d := m.deps[index]
	home := m.home
	return func() tea.Msg {
		result := deps.SyncOne(d.name, d.url, home)
		return depDoneMsg{index: index, result: result}
	}
}

func (m model) tick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(_ time.Time) tea.Msg {
		return tickMsg{}
	})
}
