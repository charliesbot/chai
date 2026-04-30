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

	AntigravityStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("135")) // purple

	OpenCodeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")) // teal

	DroidStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")) // pink

	CodexStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46")) // bright green
)

// Platform icons
func ClaudeIcon() string {
	return ClaudeStyle.Render("●")
}

func GeminiIcon() string {
	return GeminiStyle.Render("◆")
}

func AntigravityIcon() string {
	return AntigravityStyle.Render("▲")
}

func OpenCodeIcon() string {
	return OpenCodeStyle.Render("■")
}

func DroidIcon() string {
	return DroidStyle.Render("✦")
}

func CodexIcon() string {
	return CodexStyle.Render("⬢")
}

// PlatformState represents whether a platform was synced, failed, or not applicable.
type PlatformState int

const (
	PlatformOK PlatformState = iota
	PlatformFailed
	PlatformNA // not applicable
)

// PlatformStatus pairs a platform name with its sync state for UI rendering.
type PlatformStatus struct {
	Name  string
	State PlatformState
}

func PlatformIcons(statuses []PlatformStatus) string {
	parts := make([]string, len(statuses))
	for i, s := range statuses {
		parts[i] = renderPlatformState(platformIcon(s.Name), s.State)
	}
	return strings.Join(parts, " ")
}

func renderPlatformState(icon string, state PlatformState) string {
	switch state {
	case PlatformFailed:
		return Warning.Render("✗")
	case PlatformNA:
		return Muted.Render("·")
	default:
		return icon
	}
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

// Box renders a boxed section with header, platform icons, and one item per line.
//
//	┌ skills (3) ───────────────── ● ◆ ▲
//	│ agents-md
//	│ android-dev
//	│ slidev
//	└
func Box(name string, count int, statuses []PlatformStatus, items []string) string {
	icons := PlatformIcons(statuses)

	header := Label.Render(name)
	countStr := ""
	if count > 0 {
		countStr = " " + Muted.Render(fmt.Sprintf("(%d)", count))
	}

	headerText := name
	if count > 0 {
		headerText += fmt.Sprintf(" (%d)", count)
	}

	// Account for " ┌ " prefix (3) + headerText + " " + icons + " "
	lineLen := boxWidth - len(headerText) - 6
	if lineLen < 3 {
		lineLen = 3
	}
	line := strings.Repeat("─", lineLen)

	s := Border.Render(" ┌") + " " + header + countStr + " " + Border.Render(line) + " " + icons + "\n"

	for _, item := range items {
		s += Border.Render(" │") + " " + ItemStyle.Render(item) + "\n"
	}

	s += Border.Render(" └")

	return s
}

// Section is kept for backward compat with dry-run output.
func Section(name string, count int, statuses []PlatformStatus) string {
	countStr := ""
	if count > 0 {
		countStr = " " + Muted.Render(fmt.Sprintf("(%d)", count))
	}
	return fmt.Sprintf("%s%s  %s", Label.Render(name), countStr, PlatformIcons(statuses))
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
	case "Antigravity":
		return AntigravityIcon()
	case "OpenCode":
		return OpenCodeIcon()
	case "Droid":
		return DroidIcon()
	case "Codex":
		return CodexIcon()
	default:
		return "○"
	}
}
