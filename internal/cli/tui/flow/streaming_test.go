package flow_test

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func streamView(steps []component.Step, final *string, th theme.Theme) string {
	out := component.Steps(steps, th)
	if final != nil {
		out += component.Outcome(
			component.Result{OK: true, Subject: *final, Message: "done"}, th,
		)
	}

	return out
}

func newStream(t *testing.T, events ...flow.Event[string]) *flow.Streaming[string] {
	t.Helper()

	ch := make(chan flow.Event[string], len(events)+1)
	for _, e := range events {
		ch <- e
	}

	close(ch)

	return flow.NewStreaming(newTestTheme(t), flow.StreamOpts[string]{
		Label: "installing",
		Start: func() (<-chan flow.Event[string], error) { return ch, nil },
		View:  streamView,
	})
}

// pump drives the model until it quits, running each command it returns.
// The bound is a guard against a flow that never settles.
func pump(t *testing.T, m tea.Model) tui.CommandModel {
	t.Helper()

	cmd := m.Init()

	for range 100 {
		if cmd == nil {
			break
		}

		msg := runCmd(cmd)
		if msg == nil {
			break
		}

		if _, quit := msg.(tea.QuitMsg); quit {
			break
		}

		m, cmd = m.Update(msg)
	}

	cm, ok := m.(tui.CommandModel)
	require.True(t, ok, "model must satisfy tui.CommandModel")

	return cm
}

func TestStreaming_Run_RendersStepsThenOutcome(t *testing.T) {
	done := "github.com/u/r"
	m := newStream(
		t,
		flow.Event[string]{Kind: flow.EventStep, Name: "fetch source", State: component.StepDone},
		flow.Event[string]{Kind: flow.EventStep, Name: "build", State: component.StepRunning},
		flow.Event[string]{Kind: flow.EventDone, Final: &done},
	)

	model := pump(t, m)

	require.NoError(t, model.Err())

	view := model.View()
	assert.Contains(t, view, "fetch source")
	assert.Contains(t, view, "build")
	assert.Contains(t, view, "✓ github.com/u/r  done")
	assert.Equal(t, "github.com/u/r", model.Payload())
}

func TestStreaming_Run_StepStateIsUpsertedNotDuplicated(t *testing.T) {
	done := "x"
	m := newStream(
		t,
		flow.Event[string]{Kind: flow.EventStep, Name: "build", State: component.StepRunning},
		flow.Event[string]{Kind: flow.EventStep, Name: "build", State: component.StepDone},
		flow.Event[string]{Kind: flow.EventDone, Final: &done},
	)

	model := pump(t, m)

	assert.Contains(t, model.View(), "✓ 1 of 1  build",
		"a step reported twice must update in place")
}

func TestStreaming_Run_FailureMarksStepAndKeepsFrame(t *testing.T) {
	m := newStream(
		t,
		flow.Event[string]{Kind: flow.EventStep, Name: "build", State: component.StepRunning},
		flow.Event[string]{Kind: flow.EventFailed, Err: errors.New("exit status 1")},
	)

	model := pump(t, m)

	require.ErrorContains(t, model.Err(), "exit status 1")
	assert.Contains(t, model.View(), "✗ 1 of 1  build",
		"the failing step must stay visible")
	assert.Nil(t, model.Payload())
}

func TestStreaming_Run_StartErrorIsTerminal(t *testing.T) {
	m := flow.NewStreaming(newTestTheme(t), flow.StreamOpts[string]{
		Label: "installing",
		Start: func() (<-chan flow.Event[string], error) { return nil, errors.New("dial failed") },
		View:  streamView,
	})

	model := pump(t, m)

	assert.ErrorContains(t, model.Err(), "dial failed")
}

func TestStreaming_Run_ChannelClosedEarlyIsAnError(t *testing.T) {
	m := newStream(t)

	model := pump(t, m)

	assert.ErrorContains(t, model.Err(), "stream closed before completion")
}

func TestStreaming_View_SpinnerBeforeFirstStep(t *testing.T) {
	m := newStream(t)

	assert.Equal(t, "", m.View(), "spinner must not flash")

	var model tea.Model = m
	for range 2 {
		model, _ = model.Update(theme.TickMsg{})
	}

	assert.Contains(t, model.View(), "installing")
}

func TestStreaming_Update_UnknownMessagesAreIgnored(t *testing.T) {
	m := newStream(t)

	next, cmd := m.Update("irrelevant")

	assert.Nil(t, cmd)
	assert.Equal(t, "", next.View())
}

func TestStreaming_SatisfiesCommandModel(t *testing.T) {
	var _ tui.CommandModel = newStream(t)
}

func TestStreaming_CtrlCQuitsAndReportsInterrupted(t *testing.T) {
	m := flow.NewStreaming(newTestTheme(t), flow.StreamOpts[string]{
		Label: "watching",
		View:  streamView,
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	require.NotNil(t, cmd, "ctrl+c must return a command")
	assert.Equal(t, tea.Quit(), cmd(), "ctrl+c must quit the program")
	assert.ErrorIs(t, m.Err(), tui.Interrupted())
}

func TestStreaming_QQuitsAndReportsInterrupted(t *testing.T) {
	m := flow.NewStreaming(newTestTheme(t), flow.StreamOpts[string]{
		Label: "watching",
		View:  streamView,
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	require.NotNil(t, cmd, "q must return a command")
	assert.Equal(t, tea.Quit(), cmd(), "q must quit the program")
	assert.ErrorIs(t, m.Err(), tui.Interrupted())
}

func TestStreaming_Update_OtherKeyIsIgnored(t *testing.T) {
	m := flow.NewStreaming(newTestTheme(t), flow.StreamOpts[string]{
		Label: "watching",
		View:  streamView,
	})

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	assert.Nil(t, cmd, "an unrelated key must not settle the flow")
	assert.NoError(t, next.(tui.CommandModel).Err())
}
