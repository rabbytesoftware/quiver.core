package ui_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/ui"
)

// Styles are tested with color stripped — lipgloss disables ANSI when not a
// TTY, so test output is plain text.

func TestCommandHeader_ContainsBrandCommandSubject(t *testing.T) {
	out := ui.CommandHeader("install", "github.com/user/a")
	assert.Contains(t, out, "▸ quiver")
	assert.Contains(t, out, "install")
	assert.Contains(t, out, "github.com/user/a")
}

func TestCommandHeader_OmitsEmptyParts(t *testing.T) {
	out := ui.CommandHeader("", "")
	assert.Contains(t, out, "▸ quiver")
	assert.NotContains(t, out, "  \n\n")
}

func TestStateLabel_KnownStates(t *testing.T) {
	testCases := []struct {
		state string
		icon  string
	}{
		{"ready", "●"},
		{"running", "▶"},
		{"absent", "○"},
		{"installing", "↓"},
		{"updating", "↑"},
		{"uninstalling", "↓"},
		{"stopping", "◼"},
		{"draining", "◐"},
		{"detached", "⊘"},
		{"outdated", "!"},
		{"removed", "✗"},
	}
	for _, tc := range testCases {
		t.Run(tc.state, func(t *testing.T) {
			out := ui.StateLabel(tc.state)
			assert.Contains(t, out, tc.icon)
			assert.Contains(t, out, tc.state)
		})
	}
}

func TestStateLabel_UnknownStateFallsBack(t *testing.T) {
	out := ui.StateLabel("weird")
	assert.Contains(t, out, "?")
	assert.Contains(t, out, "weird")
}

func TestRenderTable_AlignsColumns(t *testing.T) {
	out := ui.RenderTable(
		[]string{"NAME", "STATE"},
		[][]string{
			{"short", "ready"},
			{"a-much-longer-name", "running"},
		},
	)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[0], "NAME")
	assert.Contains(t, lines[0], "STATE")
	// Both STATE cells start at the same column.
	assert.Equal(t, strings.Index(lines[1], "ready"), strings.Index(lines[2], "running"))
}

func TestRenderTable_EmptyRows(t *testing.T) {
	out := ui.RenderTable([]string{"A"}, nil)
	assert.Contains(t, out, "A")
}

func TestRenderBox_WrapsContent(t *testing.T) {
	out := ui.RenderBox("hello\nworld")
	assert.Contains(t, out, "hello")
	assert.Contains(t, out, "world")
	assert.Contains(t, out, "╭")
	assert.Contains(t, out, "╰")
}
