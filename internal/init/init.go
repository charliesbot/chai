package init

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

const defaultPath = "~/dotfiles/ai"

const tomlTemplate = `platforms = ["claude", "gemini", "opencode", "codex"]
instructions = "%s/instructions/AGENTS.md"

[deps]

[skills]
paths = ["%s/skills"]

[subagents]
paths = ["%s/subagents"]
`

// Run executes the interactive init flow.
func Run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	tomlPath := filepath.Join(home, "chai.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		fmt.Printf("skipped %s (already exists)\n", tomlPath)
		return nil
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

// Scaffold creates ~/chai.toml if it doesn't already exist.
// rawPath may contain ~ which is expanded using home.
func Scaffold(home, rawPath string) error {
	tomlPath := filepath.Join(home, "chai.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		fmt.Printf("skipped %s (already exists)\n", tomlPath)
		return nil
	}

	tomlContent := fmt.Sprintf(tomlTemplate, rawPath, rawPath, rawPath)
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", tomlPath, err)
	}
	fmt.Printf("created %s\n", tomlPath)

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
