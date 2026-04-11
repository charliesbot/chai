package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/deps"
	"github.com/charliesbot/chai/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Run executes the update with a Bubbletea TUI.
func Run(depMap map[string]config.Dep, extensions map[string]string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}
	return RunWithHome(depMap, extensions, home)
}

// RunWithHome executes the update with a Bubbletea TUI using the given home directory.
func RunWithHome(depMap map[string]config.Dep, extensions map[string]string, home string) error {
	if len(depMap) == 0 && len(extensions) == 0 {
		fmt.Println(ui.Muted.Render("nothing to update"))
		return nil
	}

	if len(depMap) > 0 {
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
	}

	if len(extensions) > 0 {
		if err := updateGeminiExtensions(extensions, home); err != nil {
			return err
		}
	}

	return nil
}

// updateGeminiExtensions installs missing Gemini extensions.
func updateGeminiExtensions(extensions map[string]string, home string) error {
	extDir := filepath.Join(home, ".gemini", "extensions")

	fmt.Println(ui.Title.Render("gemini extensions") + "\n")

	for name, url := range extensions {
		installed := filepath.Join(extDir, name)
		if _, err := os.Stat(installed); err == nil {
			fmt.Printf("  %s %s %s\n", ui.Check(), ui.Bold.Render(name), ui.Muted.Render("installed"))
			continue
		}

		fmt.Printf("  %s %s %s\n", spinnerStyle.Render("⠋"), ui.Bold.Render(name), ui.Muted.Render("installing..."))
		cmd := exec.Command("gemini", "extensions", "install", url, "--consent")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("installing gemini extension %q: %w", name, err)
		}
		fmt.Printf("  %s %s %s\n", ui.Check(), ui.Bold.Render(name), ui.Success.Render("installed"))
	}

	fmt.Println()
	return nil
}

var spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

// dep tracks the state of a single dependency during update.
type dep struct {
	name   string
	cfgDep config.Dep
	status string // "waiting", "updating", "building", "done", "error"
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

func newModel(depMap map[string]config.Dep, home string) model {
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
		d[i] = dep{name: name, cfgDep: depMap[name], status: status}
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
		url := ui.Muted.Render(d.cfgDep.URL)

		switch d.status {
		case "done":
			action := actionStyle(d.result)
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

func actionStyle(r *deps.Result) string {
	action := ui.Success.Render(string(r.Action))
	if r.Action == deps.ActionCurrent {
		action = ui.Muted.Render(string(r.Action))
	}
	if r.Built {
		action += " + " + ui.Success.Render("built")
	}
	return action
}

func (m model) startUpdate(index int) tea.Cmd {
	d := m.deps[index]
	home := m.home
	return func() tea.Msg {
		result := deps.SyncOne(d.name, d.cfgDep, home)
		return depDoneMsg{index: index, result: result}
	}
}

func (m model) tick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(_ time.Time) tea.Msg {
		return tickMsg{}
	})
}
