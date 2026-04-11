package sync

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// PromptFunc asks whether to overwrite a dirty file. Returns true to overwrite.
type PromptFunc func(path string) (bool, error)

// InteractivePrompt returns a PromptFunc that uses Bubbletea to ask the user.
func InteractivePrompt() PromptFunc {
	return func(path string) (bool, error) {
		m := newPromptModel(path)
		p := tea.NewProgram(m)
		final, err := p.Run()
		if err != nil {
			return false, fmt.Errorf("running prompt: %w", err)
		}
		result := final.(promptModel)
		return result.confirmed, nil
	}
}

type promptModel struct {
	path      string
	confirmed bool
	done      bool
}

func newPromptModel(path string) promptModel {
	return promptModel{path: path}
}

func (m promptModel) Init() tea.Cmd {
	return nil
}

func (m promptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			m.confirmed = true
			m.done = true
			return m, tea.Quit
		case "n", "N", "ctrl+c", "esc":
			m.confirmed = false
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m promptModel) View() string {
	if m.done {
		return ""
	}
	return fmt.Sprintf("%s was modified since last sync. Overwrite? [y/n] ", m.path)
}
