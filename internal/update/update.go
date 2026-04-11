package update

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charliesbot/chai/internal/config"
	"github.com/charliesbot/chai/internal/deps"
	"github.com/charliesbot/chai/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

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

	m := newModel(depMap, extensions, home)
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

// itemKind distinguishes deps from extensions.
type itemKind int

const (
	kindDep itemKind = iota
	kindExtension
)

// item tracks the state of a single dep or extension during update.
type item struct {
	name   string
	url    string
	kind   itemKind
	dep    *config.Dep // only for deps
	status string      // "waiting", "updating", "done", "error"
	action string      // result description
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

func newModel(depMap map[string]config.Dep, extensions map[string]string, home string) model {
	var items []item

	// Add deps first
	depNames := sortedKeys(depMap)
	for _, name := range depNames {
		dep := depMap[name]
		items = append(items, item{
			name:   name,
			url:    dep.URL,
			kind:   kindDep,
			dep:    &dep,
			status: "waiting",
		})
	}

	// Add extensions
	extNames := sortedStringKeys(extensions)
	for _, name := range extNames {
		items = append(items, item{
			name:   name,
			url:    extensions[name],
			kind:   kindExtension,
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
	// Check if we have deps and/or extensions
	hasDeps := false
	hasExts := false
	for _, it := range m.items {
		if it.kind == kindDep {
			hasDeps = true
		} else {
			hasExts = true
		}
	}

	s := ""

	if hasDeps {
		s += ui.Title.Render("deps") + "\n\n"
		for _, it := range m.items {
			if it.kind != kindDep {
				continue
			}
			s += m.renderItem(it)
		}
		s += "\n"
	}

	if hasExts {
		s += ui.Title.Render("gemini extensions") + "\n\n"
		for _, it := range m.items {
			if it.kind != kindExtension {
				continue
			}
			s += m.renderItem(it)
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
		action := ui.Success.Render(it.action)
		if it.action == "up to date" || it.action == "installed" {
			action = ui.Muted.Render(it.action)
		}
		return fmt.Sprintf("  %s %s %s %s\n", icon, name, url, action)
	case "error":
		return fmt.Sprintf("  %s %s %s %s\n", icon, name, url, ui.Warning.Render("error"))
	default:
		return fmt.Sprintf("  %s %s %s\n", icon, name, url)
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
			result := deps.SyncOne(it.name, *it.dep, home)
			if result.Err != nil {
				return itemDoneMsg{index: index, err: result.Err}
			}
			action := string(result.Action)
			if result.Built {
				action += " + built"
			}
			return itemDoneMsg{index: index, action: action}

		case kindExtension:
			cmd := exec.Command("gemini", "extensions", "install", it.url, "--consent")
			out, err := cmd.CombinedOutput()
			if err != nil {
				// "already installed" is not a real error
				if strings.Contains(string(out), "already installed") {
					return itemDoneMsg{index: index, action: "installed"}
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

func sortedKeys(m map[string]config.Dep) []string {
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
