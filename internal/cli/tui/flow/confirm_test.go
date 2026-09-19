package flow_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
)

func key(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}

	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestConfirm_View_ShowsPromptBeforeDeciding(t *testing.T) {
	th := newTestTheme(t)
	m := flow.Confirm(th, "remove github.com/u/r?",
		newCounter(th, func() (int, error) { return 1, nil }))

	assert.Contains(t, m.View(), "remove github.com/u/r?")
	assert.Nil(t, m.Init(), "confirm must not start the inner model before deciding")
}

func TestConfirm_Update_AcceptDelegatesToNext(t *testing.T) {
	th := newTestTheme(t)
	inner := newCounter(th, func() (int, error) { return 4, nil })
	m := flow.Confirm(th, "proceed?", inner)

	accepted, cmd := m.Update(key("y"))
	require.NotNil(t, cmd, "accepting must start the inner model")

	settled, _ := accepted.Update(fetchResult(t, inner))
	model, ok := settled.(tui.CommandModel)
	require.True(t, ok)

	assert.NoError(t, model.Err())
	assert.Equal(t, 4, model.Payload())
	assert.Equal(t, "count 4\n", model.View())
}

func TestConfirm_Update_EnterAccepts(t *testing.T) {
	th := newTestTheme(t)
	m := flow.Confirm(th, "proceed?", newCounter(th, func() (int, error) { return 1, nil }))

	_, cmd := m.Update(key("enter"))

	assert.NotNil(t, cmd)
}

func TestConfirm_Update_AbortIsSuccessWithNoPayload(t *testing.T) {
	th := newTestTheme(t)
	m := flow.Confirm(th, "proceed?", newCounter(th, func() (int, error) {
		t.Fatal("inner model must not run when aborted")

		return 0, nil
	}))

	aborted, cmd := m.Update(key("n"))
	model, ok := aborted.(tui.CommandModel)
	require.True(t, ok)

	require.NotNil(t, cmd, "aborting must quit")
	assert.NoError(t, model.Err(), "aborting is not a failure")
	assert.Nil(t, model.Payload())
	assert.Equal(t, "", model.View())
}

func TestConfirm_Update_IgnoresUnrelatedKeysAndMessages(t *testing.T) {
	th := newTestTheme(t)
	m := flow.Confirm(th, "proceed?", newCounter(th, func() (int, error) { return 1, nil }))

	next, cmd := m.Update(key("q"))
	assert.Nil(t, cmd)
	assert.Contains(t, next.View(), "proceed?")

	next, cmd = next.Update("not a key")
	assert.Nil(t, cmd)
	assert.Contains(t, next.View(), "proceed?")
}

func TestConfirmGuard_RequiresYesWhenNotATTY(t *testing.T) {
	testCases := []struct {
		name    string
		tty     bool
		yes     bool
		wantErr bool
	}{
		{"tty prompts", true, false, false},
		{"yes skips the prompt", false, true, false},
		{"piped without yes is a usage error", false, false, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := flow.ConfirmGuard(tc.tty, tc.yes)

			if !tc.wantErr {
				assert.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Equal(t, tui.ExitUsage, tui.CodeFor(err))
		})
	}
}
