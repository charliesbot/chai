package init

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const defaultPath = "~/dotfiles/ai"

const tomlTemplate = `instructions = "%s/agents.md"

[deps]

[skills]
paths = []

[agents]
paths = []
`

const agentsTemplate = `# AI Agent Instructions

Add your shared instructions here. This file will be synced to all platforms by chai.
`

// Run executes the interactive init flow.
func Run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	tomlPath := filepath.Join(home, "chai.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		return fmt.Errorf("%s already exists — remove it first to re-initialize", tomlPath)
	}

	m := newModel()
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("running prompt: %w", err)
	}

	result := finalModel.(model)
	if result.cancelled {
		fmt.Println("init cancelled")
		return nil
	}

	rawPath := result.textInput.Value()
	if rawPath == "" {
		rawPath = defaultPath
	}

	return Scaffold(home, rawPath)
}

// Scaffold creates the chai.toml and agents.md files.
// rawPath may contain ~ which is expanded using home.
func Scaffold(home, rawPath string) error {
	tomlPath := filepath.Join(home, "chai.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		return fmt.Errorf("%s already exists — remove it first to re-initialize", tomlPath)
	}

	expandedPath := rawPath
	if strings.HasPrefix(expandedPath, "~/") {
		expandedPath = filepath.Join(home, expandedPath[2:])
	} else if expandedPath == "~" {
		expandedPath = home
	}

	if err := os.MkdirAll(expandedPath, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", expandedPath, err)
	}

	agentsPath := filepath.Join(expandedPath, "agents.md")
	if err := os.WriteFile(agentsPath, []byte(agentsTemplate), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", agentsPath, err)
	}

	tomlContent := fmt.Sprintf(tomlTemplate, rawPath)
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", tomlPath, err)
	}

	fmt.Printf("created %s\n", tomlPath)
	fmt.Printf("created %s\n", agentsPath)

	return nil
}

type model struct {
	textInput textinput.Model
	done      bool
	cancelled bool
}

func newModel() model {
	ti := textinput.New()
	ti.Placeholder = defaultPath
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50

	return model{textInput: ti}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.cancelled = true
			return m, tea.Quit
		case tea.KeyEnter:
			m.done = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.done || m.cancelled {
		return ""
	}
	return fmt.Sprintf("Where should chai store your AI config?\n\n%s\n\n(press Enter for %s)\n", m.textInput.View(), defaultPath)
}
