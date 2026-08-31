package flow_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func newTestTheme(t *testing.T) theme.Theme {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "0")

	var buf bytes.Buffer

	return theme.New(lipgloss.NewRenderer(&buf))
}

// runCmd executes cmd, unwrapping a tea.Batch and skipping spinner ticks.
// tea.BatchMsg is []tea.Cmd, so a batch must be run element by element.
func runCmd(cmd tea.Cmd) tea.Msg {
	msg := cmd()

	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}

	for _, c := range batch {
		got := c()
		if got == nil {
			continue
		}

		if _, isTick := got.(theme.TickMsg); isTick {
			continue
		}

		return got
	}

	return nil
}

// fetchResult runs the model's Init command and returns the message it emits.
func fetchResult(t *testing.T, m tea.Model) tea.Msg {
	t.Helper()

	msg := runCmd(m.Init())
	require.NotNil(t, msg, "Init produced no non-tick message")

	return msg
}

func newCounter(th theme.Theme, fetch func() (int, error)) *flow.Instant[int] {
	return flow.NewInstant(th, "loading", fetch,
		func(n int, _ theme.Theme) string { return fmt.Sprintf("count %d\n", n) })
}

func TestInstant_Update_ReadyStoresDataAndQuits(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 7, nil })

	next, quit := m.Update(fetchResult(t, m))
	model, ok := next.(tui.CommandModel)
	require.True(t, ok)

	require.NotNil(t, quit, "a terminal result must quit the program")
	assert.NoError(t, model.Err())
	assert.Equal(t, 7, model.Payload())
	assert.Equal(t, "count 7\n", model.View())
}

func TestInstant_Update_FetchErrorIsTerminal(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 0, errors.New("not found") })

	next, quit := m.Update(fetchResult(t, m))
	model, ok := next.(tui.CommandModel)
	require.True(t, ok)

	require.NotNil(t, quit)
	assert.ErrorContains(t, model.Err(), "not found")
	assert.Equal(t, "", model.View(), "a failed instant run renders nothing")
}

func TestInstant_View_SpinnerAppearsOnlyAfterDelay(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 1, nil })

	assert.Equal(t, "", m.View(), "spinner must not flash")

	var model tea.Model = m
	for range 2 {
		model, _ = model.Update(theme.TickMsg(time.Now()))
	}

	assert.Contains(t, model.View(), "loading")
}

func TestInstant_Update_TicksStopOnceSettled(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 1, nil })

	settled, _ := m.Update(fetchResult(t, m))
	_, cmd := settled.Update(theme.TickMsg(time.Now()))

	assert.Nil(t, cmd, "the tick chain must not be re-armed after settling")
}

func TestInstant_Update_UnknownMessagesAreIgnored(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 1, nil })

	next, cmd := m.Update("irrelevant")

	assert.Nil(t, cmd)
	assert.Equal(t, "", next.View())
}

func TestInstant_Init_ArmsSpinnerAndFetch(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 1, nil })

	assert.NotNil(t, m.Init())
}

func TestInstant_SatisfiesCommandModel(t *testing.T) {
	var _ tui.CommandModel = newCounter(newTestTheme(t), func() (int, error) { return 0, nil })
}

func TestInstant_CtrlCQuitsAndReportsInterrupted(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 1, nil })

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	require.NotNil(t, cmd, "ctrl+c must return a command")
	assert.Equal(t, tea.Quit(), cmd(), "ctrl+c must quit the program")
	assert.ErrorIs(t, m.Err(), tui.Interrupted())
}

func TestInstant_QQuitsAndReportsInterrupted(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 1, nil })

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	require.NotNil(t, cmd, "q must return a command")
	assert.Equal(t, tea.Quit(), cmd(), "q must quit the program")
	assert.ErrorIs(t, m.Err(), tui.Interrupted())
}

func TestInstant_Update_OtherKeyIsIgnored(t *testing.T) {
	th := newTestTheme(t)
	m := newCounter(th, func() (int, error) { return 1, nil })

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	assert.Nil(t, cmd, "an unrelated key must not settle the flow")
	assert.NoError(t, next.(tui.CommandModel).Err())
}
