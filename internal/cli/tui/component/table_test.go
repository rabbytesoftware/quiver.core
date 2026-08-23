package component_test

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func newTestTheme(t *testing.T) theme.Theme {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "0")

	var buf bytes.Buffer

	return theme.New(lipgloss.NewRenderer(&buf))
}

func TestTable_Render_PadsColumnsToWidestCell(t *testing.T) {
	th := newTestTheme(t)
	cols := []component.Column{{Title: "NAME"}, {Title: "STATE"}}
	rows := [][]string{{"repo", "ready"}, {"much-longer-name", "running"}}

	got := component.Table(cols, rows, "none", th)

	assert.Equal(t, ""+
		"NAME              STATE\n"+
		"repo              ready\n"+
		"much-longer-name  running\n", got)
}

func TestTable_Render_EmptyRowsRendersEmptyMessageNotHeaders(t *testing.T) {
	th := newTestTheme(t)
	cols := []component.Column{{Title: "NAME"}, {Title: "STATE"}}

	got := component.Table(cols, nil, "no arrows match -F ffmpeg", th)

	assert.Equal(t, "no arrows match -F ffmpeg\n", got)
	assert.NotContains(t, got, "NAME", "must not print headers over nothing")
}

func TestTable_Render_NoColumnsRendersEmptyMessage(t *testing.T) {
	th := newTestTheme(t)

	got := component.Table(nil, [][]string{{"x"}}, "nothing to show", th)

	assert.Equal(t, "nothing to show\n", got)
}

func TestTable_Render_ShortRowsArePadded(t *testing.T) {
	th := newTestTheme(t)
	cols := []component.Column{{Title: "A"}, {Title: "B"}}

	got := component.Table(cols, [][]string{{"only"}}, "none", th)

	assert.Equal(t, "A     B\nonly  \n", got)
}

func TestTable_Render_ExtraCellsAreTruncated(t *testing.T) {
	th := newTestTheme(t)
	cols := []component.Column{{Title: "A"}}

	got := component.Table(cols, [][]string{{"x", "dropped"}}, "none", th)

	assert.Equal(t, "A\nx\n", got)
	assert.NotContains(t, got, "dropped")
}

func TestTable_Render_WideRunesAreMeasuredByDisplayWidth(t *testing.T) {
	th := newTestTheme(t)
	cols := []component.Column{{Title: "GLYPH"}, {Title: "NAME"}}

	got := component.Table(cols, [][]string{{"●", "ready"}}, "none", th)

	assert.Equal(t, "GLYPH  NAME\n●      ready\n", got)
}
