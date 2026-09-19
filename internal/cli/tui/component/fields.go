package component

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// emptyValue marks a field that is present but carries no value.
const emptyValue = "—"

// Field is one label/value pair. Set distinguishes a field that is absent from
// one that is present but empty.
type Field struct {
	Label string
	Value string
	Set   bool
}

// Fields renders label/value pairs beneath an optional title. Fields that are
// not set are omitted; fields that are set but empty render an em dash, so two
// entities never differ in row count with no signal as to why.
func Fields(title string, fields []Field, t theme.Theme) string {
	shown, width := shownFields(fields)
	if len(shown) == 0 {
		return ""
	}

	var b strings.Builder

	if title != "" {
		b.WriteString(t.Header.Render(title) + "\n")
	}

	for _, f := range shown {
		value := f.Value
		if value == "" {
			value = t.Muted.Render(emptyValue)
		}

		pad := strings.Repeat(" ", width-lipgloss.Width(f.Label)+colGap)
		b.WriteString(t.Label.Render(f.Label) + pad + value + "\n")
	}

	return b.String()
}

// shownFields returns the fields that render, and the width of the widest of
// their labels. Hidden labels must not widen the column.
func shownFields(fields []Field) ([]Field, int) {
	shown := make([]Field, 0, len(fields))
	width := 0

	for _, f := range fields {
		if !f.Set {
			continue
		}

		shown = append(shown, f)

		if w := lipgloss.Width(f.Label); w > width {
			width = w
		}
	}

	return shown, width
}
