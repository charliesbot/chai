package ui

import (
	"fmt"
	"strings"

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

	// Platform colors
	ClaudeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")) // orange

	GeminiStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("75")) // blue
)

// Platform icons
func ClaudeIcon() string {
	return ClaudeStyle.Render("●")
}

func GeminiIcon() string {
	return GeminiStyle.Render("◆")
}

func PlatformIcons(claudeOk, geminiOk bool) string {
	c := ClaudeIcon()
	if !claudeOk {
		c = Warning.Render("✗")
	}
	g := GeminiIcon()
	if !geminiOk {
		g = Warning.Render("✗")
	}
	return c + " " + g
}

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

// Section renders a category header with platform status indicators.
func Section(name string, count int, claudeOk, geminiOk bool) string {
	countStr := ""
	if count > 0 {
		countStr = " " + Muted.Render(fmt.Sprintf("(%d)", count))
	}
	return fmt.Sprintf("%s%s  %s", Label.Render(name), countStr, PlatformIcons(claudeOk, geminiOk))
}

// ItemList renders a comma-separated list of item names, indented.
func ItemList(names []string) string {
	styled := make([]string, len(names))
	for i, n := range names {
		styled[i] = Bold.Render(n)
	}
	return "  " + strings.Join(styled, Muted.Render(", "))
}

// SyncedLine is kept for dry-run detail lines.
func SyncedLine(platform, path string) string {
	icon := platformIcon(platform)
	return fmt.Sprintf("  %s %s %s", icon, Bold.Render(platform), Muted.Render(path))
}

func SkippedLine(platform, path string) string {
	icon := platformIcon(platform)
	return fmt.Sprintf("  %s %s %s %s", icon, Bold.Render(platform), Muted.Render(path), Warning.Render("skipped"))
}

func DryRunLine(platform, path string) string {
	icon := platformIcon(platform)
	return fmt.Sprintf("  %s %s %s", icon, Bold.Render(platform), Muted.Render(path))
}

func platformIcon(name string) string {
	switch name {
	case "Claude":
		return ClaudeIcon()
	case "Gemini":
		return GeminiIcon()
	default:
		return "○"
	}
}
