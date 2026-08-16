package tui_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
)

// fakeModel is a CommandModel that quits immediately with a fixed result.
type fakeModel struct {
	view    string
	payload any
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

// bareModel is a tea.Model that is deliberately not a CommandModel.
type bareModel struct{}

func (bareModel) Init() tea.Cmd                       { return tea.Quit }
func (bareModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return bareModel{}, tea.Quit }
func (bareModel) View() string                        { return "" }

// morphModel starts as a CommandModel but replaces itself with one that is not,
// which is the only way to reach Run's final-model type guard. Init must emit a
// non-quit message first: bubbletea short-circuits on a QuitMsg from Init and
// would never call Update.
type morphModel struct{ fakeModel }

func (m *morphModel) Init() tea.Cmd {
	return func() tea.Msg { return "morph" }
}

func (m *morphModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return bareModel{}, tea.Quit }

// failWriter fails every write, exercising the output error paths.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

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

// The TTY + FormatTable branch of Runner.write is deliberately not covered by a
// unit test. Reaching it requires bubbletea to open /dev/tty, which is absent in
// CI and in containers, so the test would be environment-dependent rather than
// meaningful. The branch is a single `return nil`, exercised by any real
// terminal run.

func TestRunner_Run_FinalModelMustBeACommandModel(t *testing.T) {
	var buf bytes.Buffer

	r := tui.NewRunner(&buf, tui.FormatJSON, false)

	err := r.Run(context.Background(), &morphModel{})

	require.ErrorContains(t, err, "is not a CommandModel")
	assert.Equal(t, tui.ExitFailure, tui.CodeFor(err))
}

func TestRunner_Run_EncodeAndWriteFailuresAreReported(t *testing.T) {
	unserializable := func() *fakeModel {
		m := newFake()
		m.payload = make(chan int)

		return m
	}

	testCases := []struct {
		name   string
		format tui.Format
		out    io.Writer
		model  *fakeModel
		want   string
	}{
		{"json encode", tui.FormatJSON, &bytes.Buffer{}, unserializable(), "encode json"},
		{"yaml panics on a bad type", tui.FormatYAML, &bytes.Buffer{}, unserializable(), "encode yaml"},
		{"yaml errors on a bad writer", tui.FormatYAML, failWriter{}, newFake(), "encode yaml"},
		{"table write", tui.FormatTable, failWriter{}, newFake(), "write output"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := tui.NewRunner(tc.out, tc.format, false)

			err := r.Run(context.Background(), tc.model)

			require.ErrorContains(t, err, tc.want)
		})
	}
}
