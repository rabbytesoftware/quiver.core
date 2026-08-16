package component

import (
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

// Result is the terminal outcome of a command.
type Result struct {
	OK      bool
	Subject string
	Message string
}

// Outcome renders a single-line terminal result.
func Outcome(r Result, t theme.Theme) string {
	glyph := t.Fail.Render("✗")
	if r.OK {
		glyph = t.OK.Render("✓")
	}

	var b strings.Builder

	b.WriteString(glyph + " " + r.Subject)

	if r.Message != "" {
		b.WriteString(strings.Repeat(" ", colGap) + r.Message)
	}

	b.WriteString("\n")

	return b.String()
}
