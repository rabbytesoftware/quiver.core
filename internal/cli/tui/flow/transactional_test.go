package flow_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func TestTransactional_Update_ReportsOutcome(t *testing.T) {
	th := newTestTheme(t)
	m := flow.NewTransactional(th, flow.TxOpts[string]{
		Label: "adding arrow",
		Do:    func() (string, error) { return "github.com/u/r", nil },
		View: func(ns string, vt theme.Theme) string {
			return component.Outcome(
				component.Result{OK: true, Subject: ns, Message: "added"}, vt,
			)
		},
	})

	next, quit := m.Update(fetchResult(t, m))
	model, ok := next.(tui.CommandModel)
	require.True(t, ok)

	require.NotNil(t, quit)
	assert.NoError(t, model.Err())
	assert.Equal(t, "✓ github.com/u/r  added\n", model.View())
	assert.Equal(t, "github.com/u/r", model.Payload())
}

func TestTransactional_Update_MutationErrorIsTerminal(t *testing.T) {
	th := newTestTheme(t)
	m := flow.NewTransactional(th, flow.TxOpts[string]{
		Label: "adding arrow",
		Do:    func() (string, error) { return "", errors.New("already exists") },
		View:  func(string, theme.Theme) string { return "unreachable" },
	})

	next, _ := m.Update(fetchResult(t, m))
	model, ok := next.(tui.CommandModel)
	require.True(t, ok)

	assert.ErrorContains(t, model.Err(), "already exists")
	assert.Equal(t, "", model.View())
}

func TestTransactional_SatisfiesCommandModel(t *testing.T) {
	var _ tui.CommandModel = flow.NewTransactional(newTestTheme(t), flow.TxOpts[int]{
		Do:   func() (int, error) { return 0, nil },
		View: func(int, theme.Theme) string { return "" },
	})
}
