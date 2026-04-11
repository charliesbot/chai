package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const boxWidth = 50

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

	Border = lipgloss.NewStyle().
		Foreground(lipgloss.Color("238")) // dark gray

	ItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")) // light

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

// Box renders a boxed section with header, platform icons, and item lines.
//
//	┌ skills (12) ──────────────── ● ◆
//	│ agents-md  android-dev  slidev
//	│ web-dev  angular-developer
//	└
func Box(name string, count int, claudeOk, geminiOk bool, items []string) string {
	icons := PlatformIcons(claudeOk, geminiOk)

	// Build header: ┌ name (count) ─── ● ◆
	header := Label.Render(name)
	countStr := ""
	if count > 0 {
		countStr = " " + Muted.Render(fmt.Sprintf("(%d)", count))
	}

	headerText := name
	if count > 0 {
		headerText += fmt.Sprintf(" (%d)", count)
	}

	// Calculate padding for the line
	// Account for " ┌ " prefix (3) + headerText + " " + icons + " "
	lineLen := boxWidth - len(headerText) - 6
	if lineLen < 3 {
		lineLen = 3
	}
	line := strings.Repeat("─", lineLen)

	s := Border.Render(" ┌") + " " + header + countStr + " " + Border.Render(line) + " " + icons + "\n"

	// Render items
	if len(items) > 0 {
		lines := wrapItems(items, boxWidth-4) // 4 for " │ " prefix + margin
		for _, l := range lines {
			s += Border.Render(" │") + " " + l + "\n"
		}
	}

	s += Border.Render(" └")

	return s
}

// wrapItems arranges items into lines that fit within maxWidth.
func wrapItems(items []string, maxWidth int) []string {
	var lines []string
	var current []string
	currentLen := 0

	for _, item := range items {
		itemLen := len(item)
		sepLen := 2

		if currentLen > 0 && currentLen+sepLen+itemLen > maxWidth {
			lines = append(lines, renderItemLine(current))
			current = nil
			currentLen = 0
		}

		if currentLen > 0 {
			currentLen += sepLen
		}
		current = append(current, item)
		currentLen += itemLen
	}

	if len(current) > 0 {
		lines = append(lines, renderItemLine(current))
	}

	return lines
}

func renderItemLine(items []string) string {
	styled := make([]string, len(items))
	for i, item := range items {
		styled[i] = ItemStyle.Render(item)
	}
	return strings.Join(styled, Muted.Render("  "))
}

// Section is kept for backward compat with dry-run output.
func Section(name string, count int, claudeOk, geminiOk bool) string {
	countStr := ""
	if count > 0 {
		countStr = " " + Muted.Render(fmt.Sprintf("(%d)", count))
	}
	return fmt.Sprintf("%s%s  %s", Label.Render(name), countStr, PlatformIcons(claudeOk, geminiOk))
}

// ItemList is kept for backward compat with dry-run output.
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
