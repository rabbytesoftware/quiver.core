package tui_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

// fakeModel is a CommandModel that quits immediately with a fixed result.
type fakeModel struct {
	view    string
	payload map[string]string
	err     error
}

func (m *fakeModel) Init() tea.Cmd                       { return tea.Quit }
func (m *fakeModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, tea.Quit }
func (m *fakeModel) View() string                        { return m.view }
func (m *fakeModel) Payload() any                        { return m.payload }
func (m *fakeModel) Err() error                          { return m.err }

func newFake() *fakeModel {
	return &fakeModel{view: "NAME\nrepo\n", payload: map[string]string{"name": "repo"}}
}

func TestParseFormat_KnownAndUnknown(t *testing.T) {
	testCases := []struct {
		name    string
		in      string
		want    tui.Format
		wantErr bool
	}{
		{"table", "table", tui.FormatTable, false},
		{"json", "json", tui.FormatJSON, false},
		{"yaml", "yaml", tui.FormatYAML, false},
		{"unknown", "xml", tui.FormatTable, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tui.ParseFormat(tc.in)

			if tc.wantErr {
				require.Error(t, err)
				assert.Equal(t, tui.ExitUsage, tui.CodeFor(err))

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRunner_Run_WritesPerFormat(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "0")

	testCases := []struct {
		name   string
		format tui.Format
		want   string
	}{
		{"piped table writes the view", tui.FormatTable, "NAME\nrepo\n"},
		{"json encodes the payload", tui.FormatJSON, "{\n  \"name\": \"repo\"\n}\n"},
		{"yaml encodes the payload", tui.FormatYAML, "name: repo\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer

			r := tui.NewRunner(&buf, tc.format, false)

			require.NoError(t, r.Run(context.Background(), newFake()))
			assert.Equal(t, tc.want, buf.String())
		})
	}
}

func TestRunner_Run_ModelErrorIsReturnedAndSuppressesPayload(t *testing.T) {
	var buf bytes.Buffer

	r := tui.NewRunner(&buf, tui.FormatJSON, false)

	m := newFake()
	m.err = errors.New("not found")

	err := r.Run(context.Background(), m)

	require.ErrorContains(t, err, "not found")
	assert.Empty(t, buf.String(), "no result may be written when the command failed")
}

func TestRunner_Run_FailedTableRunKeepsItsFrame(t *testing.T) {
	var buf bytes.Buffer

	r := tui.NewRunner(&buf, tui.FormatTable, false)

	m := newFake()
	m.view = "✗ 2 of 7  build\n"
	m.err = errors.New("build failed")

	require.Error(t, r.Run(context.Background(), m))
	assert.Equal(t, "✗ 2 of 7  build\n", buf.String(), "the trace of where it died must survive")
}

func TestRunner_Theme_IsUsable(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer

	assert.Equal(t, "x", tui.NewRunner(&buf, tui.FormatTable, false).Theme().Muted.Render("x"))
}

func TestRunner_Run_CancelledContextIsReported(t *testing.T) {
	var buf bytes.Buffer

	r := tui.NewRunner(&buf, tui.FormatJSON, false)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.Error(t, r.Run(ctx, newFake()))
}
