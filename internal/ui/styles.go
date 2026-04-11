package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212")) // purple

	Success = lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")) // green

	Warning = lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")) // orange

	Muted = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")) // gray

	Bold = lipgloss.NewStyle().
		Bold(true)

	Label = lipgloss.NewStyle().
		Foreground(lipgloss.Color("75")). // blue
		Bold(true)

	DryRun = lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")). // orange
		Bold(true)

	JSONBlock = lipgloss.NewStyle().
			Foreground(lipgloss.Color("251")). // light gray
			PaddingLeft(2)
)

func DryRunTag() string {
	return DryRun.Render("[dry-run]")
}

func Arrow() string {
	return Muted.Render("→")
}

func Check() string {
	return Success.Render("✓")
}

func Skip() string {
	return Warning.Render("⊘")
}

func SyncedLine(platform, path string) string {
	return fmt.Sprintf("  %s %s %s", Check(), Bold.Render(platform), Muted.Render(path))
}

func SkippedLine(platform, path string) string {
	return fmt.Sprintf("  %s %s %s", Skip(), Bold.Render(platform), Muted.Render(path))
}

func DryRunLine(platform, path string) string {
	return fmt.Sprintf("  %s %s %s", Arrow(), Bold.Render(platform), Muted.Render(path))
}
