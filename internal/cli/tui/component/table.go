// Package component holds the CLI's pure view components. Every function here
// is a total function of its data and theme, with no state and no I/O.
package component

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// colGap is the number of spaces separating adjacent columns.
const colGap = 2

// Column is a table heading.
type Column struct {
	Title string
}

// Table renders rows beneath cols. When rows is empty it renders empty instead
// of a header row, so that an empty result and a filtered-out result differ.
func Table(cols []Column, rows [][]string, empty string, t theme.Theme) string {
	if len(cols) == 0 || len(rows) == 0 {
		return t.Muted.Render(empty) + "\n"
	}

	widths := columnWidths(cols, rows)

	titles := make([]string, len(cols))
	for i, c := range cols {
		titles[i] = c.Title
	}

	var b strings.Builder

	b.WriteString(t.Header.Render(joinCells(titles, widths)) + "\n")

	for _, row := range rows {
		b.WriteString(joinCells(row, widths) + "\n")
	}

	return b.String()
}

func columnWidths(cols []Column, rows [][]string) []int {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = lipgloss.Width(c.Title)
	}

	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}

			if w := lipgloss.Width(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}

	return widths
}

// joinCells pads cells to widths, dropping extras and padding short rows. The
// final column is not padded, so lines carry no trailing run of spaces.
func joinCells(cells []string, widths []int) string {
	var b strings.Builder

	for i, w := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}

		b.WriteString(cell)

		if i == len(widths)-1 {
			break
		}

		b.WriteString(strings.Repeat(" ", w-lipgloss.Width(cell)+colGap))
	}

	return b.String()
}
