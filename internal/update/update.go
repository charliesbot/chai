package update

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/deps"
	"github.com/charliesbot/chai/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Run executes the update with a Bubbletea TUI.
func Run(depMap map[string]config.Dep) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	return RunWithHome(depMap, home)
}

// RunWithHome executes the update with a Bubbletea TUI using the given home directory.
func RunWithHome(depMap map[string]config.Dep, home string) error {
	if len(depMap) == 0 {
		fmt.Println(ui.Muted.Render("nothing to update"))
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

// item tracks the state of a single dep during update.
type item struct {
	name   string
	dep    config.Dep
	status string // "waiting", "updating", "done", "error"
	action string // result description
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

func newModel(depMap map[string]config.Dep, home string) model {
	var items []item

	depNames := sortedKeys(depMap)
	for _, name := range depNames {
		items = append(items, item{
			name:   name,
			dep:    depMap[name],
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
	if len(m.items) == 0 {
		return ""
	}
	s := ui.Title.Render("deps") + "\n\n"
	for _, it := range m.items {
		s += m.renderItem(it)
	}
	return s + "\n"
}

func (m model) renderItem(it item) string {
	icon := m.statusIcon(it.status)
	name := ui.Bold.Render(it.name)
	url := ui.Muted.Render(it.dep.URL)

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
		result := deps.SyncOne(it.name, it.dep, home)
		if result.Err != nil {
			return itemDoneMsg{index: index, err: result.Err}
		}
		action := string(result.Action)
		if result.Built {
			action += " + built"
		}
		return itemDoneMsg{index: index, action: action}
	}
}

func (m model) tick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(_ time.Time) tea.Msg {
		return tickMsg{}
	})
}

func sortedKeys(m map[string]config.Dep) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
