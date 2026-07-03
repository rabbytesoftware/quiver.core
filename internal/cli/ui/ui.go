// Package ui holds the CLI's visual design language: the color palette,
// state labels, the command header, and small layout helpers. Lipgloss
// degrades to plain text automatically when stdout is not a TTY.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette per docs/manual — amber brand, green success, indigo running.
var (
	Amber  = lipgloss.Color("#F59E0B")
	Green  = lipgloss.Color("#4ADE80")
	Red    = lipgloss.Color("#F87171")
	Blue   = lipgloss.Color("#60A5FA")
	Indigo = lipgloss.Color("#818CF8")
	Orange = lipgloss.Color("#FB923C")
	Yellow = lipgloss.Color("#FBBF24")
	White  = lipgloss.Color("#F9FAFB")
	LtGray = lipgloss.Color("#9CA3AF")
	Gray   = lipgloss.Color("#6B7280")
	DkGray = lipgloss.Color("#374151")
	Dim    = lipgloss.Color("#4B5563")
)

// Base styles shared across commands.
var (
	Brand   = lipgloss.NewStyle().Foreground(Amber).Bold(true)
	Bold    = lipgloss.NewStyle().Foreground(White).Bold(true)
	Muted   = lipgloss.NewStyle().Foreground(LtGray)
	Faint   = lipgloss.NewStyle().Foreground(Dim)
	Success = lipgloss.NewStyle().Foreground(Green)
	Failure = lipgloss.NewStyle().Foreground(Red)
	Warn    = lipgloss.NewStyle().Foreground(Yellow)
	Info    = lipgloss.NewStyle().Foreground(Blue)
)

var stateIcons = map[string]struct {
	icon  string
	color lipgloss.Color
}{
	"ready":        {"●", Green},
	"running":      {"▶", Indigo},
	"absent":       {"○", Gray},
	"installing":   {"↓", Amber},
	"updating":     {"↑", Amber},
	"uninstalling": {"↓", Amber},
	"stopping":     {"◼", Yellow},
	"draining":     {"◐", Orange},
	"detached":     {"⊘", Gray},
	"outdated":     {"!", Yellow},
	"removed":      {"✗", Red},
}

// StateLabel renders an arrow state as a colored icon + label pair.
func StateLabel(state string) string {
	s, ok := stateIcons[state]
	if !ok {
		s.icon, s.color = "?", Gray
	}
	return lipgloss.NewStyle().Foreground(s.color).Render(s.icon + " " + state)
}

// CommandHeader renders the "▸ quiver  <cmd>  <subject>" line every command
// output starts with.
func CommandHeader(cmd, subject string) string {
	parts := "\n  " + Brand.Render("▸ quiver")
	if cmd != "" {
		parts += "  " + Muted.Render(cmd)
	}
	if subject != "" {
		parts += "  " + Bold.Render(subject)
	}
	return parts + "\n"
}

// RenderTable renders left-aligned columns with a two-space indent.
func RenderTable(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && lipgloss.Width(cell) > widths[i] {
				widths[i] = lipgloss.Width(cell)
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("  ")
	for i, h := range headers {
		sb.WriteString(Faint.Render(pad(h, widths[i])) + "  ")
	}
	sb.WriteString("\n")
	for _, row := range rows {
		sb.WriteString("  ")
		for i, cell := range row {
			w := 0
			if i < len(widths) {
				w = widths[i]
			}
			sb.WriteString(pad(cell, w) + "  ")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// pad right-pads to a display width, accounting for ANSI sequences.
func pad(s string, width int) string {
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// RenderBox draws content inside a rounded border with a two-space indent.
func RenderBox(content string) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(DkGray).
		Padding(0, 2).
		Render(content)

	var sb strings.Builder
	for _, line := range strings.Split(box, "\n") {
		fmt.Fprintf(&sb, "  %s\n", line)
	}
	return sb.String()
}
