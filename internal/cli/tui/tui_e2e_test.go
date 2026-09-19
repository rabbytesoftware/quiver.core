package tui_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/tui"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

type arrow struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	State     string `json:"state"     yaml:"state"`
}

func TestFramework_InstantAcrossEveryFormat(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "0")

	testCases := []struct {
		name   string
		format tui.Format
		want   string
	}{
		{
			name:   "piped table",
			format: tui.FormatTable,
			want:   "NAMESPACE       STATE\ngithub.com/u/r  ready\n",
		},
		{
			name:   "json",
			format: tui.FormatJSON,
			want:   "{\n  \"namespace\": \"github.com/u/r\",\n  \"state\": \"ready\"\n}\n",
		},
		{
			name:   "yaml",
			format: tui.FormatYAML,
			want:   "namespace: github.com/u/r\nstate: ready\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer

			r := tui.NewRunner(&buf, tc.format, false)

			m := flow.NewInstant(r.Theme(), "loading catalog",
				func() (arrow, error) {
					return arrow{Namespace: "github.com/u/r", State: "ready"}, nil
				},
				func(a arrow, th theme.Theme) string {
					return component.Table(
						[]component.Column{{Title: "NAMESPACE"}, {Title: "STATE"}},
						[][]string{{a.Namespace, a.State}},
						"no arrows yet", th,
					)
				})

			require.NoError(t, r.Run(context.Background(), m))
			assert.Equal(t, tc.want, buf.String())
		})
	}
}

func TestFramework_TransactionalReportsOutcome(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer

	r := tui.NewRunner(&buf, tui.FormatTable, false)

	m := flow.NewTransactional(r.Theme(), flow.TxOpts[string]{
		Label: "adding arrow",
		Do:    func() (string, error) { return "github.com/u/r", nil },
		View: func(ns string, th theme.Theme) string {
			return component.Outcome(
				component.Result{OK: true, Subject: ns, Message: "added"}, th,
			)
		},
	})

	require.NoError(t, r.Run(context.Background(), m))
	assert.Equal(t, "✓ github.com/u/r  added\n", buf.String())
}

func TestFramework_StreamingRendersStepsThenOutcome(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer

	r := tui.NewRunner(&buf, tui.FormatTable, false)

	done := "github.com/u/r"

	ch := make(chan flow.Event[string], 3)
	ch <- flow.Event[string]{Kind: flow.EventStep, Name: "fetch source", State: component.StepDone}
	ch <- flow.Event[string]{Kind: flow.EventStep, Name: "build", State: component.StepDone}
	ch <- flow.Event[string]{Kind: flow.EventDone, Final: &done}

	close(ch)

	m := flow.NewStreaming(r.Theme(), flow.StreamOpts[string]{
		Label: "installing",
		Start: func() (<-chan flow.Event[string], error) { return ch, nil },
		View: func(steps []component.Step, final *string, th theme.Theme) string {
			out := component.Steps(steps, th)
			if final != nil {
				out += component.Outcome(
					component.Result{OK: true, Subject: *final, Message: "installed"}, th,
				)
			}

			return out
		},
	})

	require.NoError(t, r.Run(context.Background(), m))

	assert.Equal(t, ""+
		"✓ 1 of 2  fetch source\n"+
		"✓ 2 of 2  build\n"+
		"✓ github.com/u/r  installed\n", buf.String())
}

func TestFramework_EmptyResultIsDistinguishableFromFiltered(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer

	cols := []component.Column{{Title: "NAMESPACE"}}
	th := tui.NewRunner(&buf, tui.FormatTable, false).Theme()

	empty := component.Table(cols, nil, "no arrows yet", th)
	filtered := component.Table(cols, nil, "no arrows match -F ffmpeg", th)

	assert.NotEqual(t, empty, filtered,
		"the standard exists to make these two cases distinguishable")
}
