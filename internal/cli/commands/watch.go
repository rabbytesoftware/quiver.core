package commands

import (
	"context"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/lifecycle"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
)

func (a *app) watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch <namespace>",
		Short: "Stream live runtime events for an arrow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validNS(args[0]); err != nil {
				return err
			}

			return a.streamWatch(cmd, args[0])
		},
	}
}

// streamWatch attaches to an arrow's runtime channel and renders every push
// until the daemon closes the stream or the context is cancelled.
//
// It backs both `watch` and `status --watch`: the two are the same operation
// under different names, and the manual documents both.
func (a *app) streamWatch(cmd *cobra.Command, ns string) error {
	runner, err := a.runner(cmd)
	if err != nil {
		return err
	}

	cli, err := a.session(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	events, err := cli.SubscribeRuntime(ctx, ns)
	if err != nil {
		return err
	}

	model := flow.NewStreaming(runner.Theme(), flow.StreamOpts[output.Watch]{
		Label: "watching " + ns,
		Start: func() (<-chan flow.Event[output.Watch], error) {
			return translateWatch(ctx, events, ns), nil
		},
		View: viewWatch(ns),
	})

	return runner.Run(ctx, model)
}

// translateWatch converts runtime snapshots into flow events.
//
// A closed stream is this command's success condition rather than an error:
// watch has no terminal result of its own, so it reports what it saw and the
// fact that the daemon stopped pushing.
func translateWatch(
	ctx context.Context,
	events <-chan apidto.ArrowRuntimeDTO,
	ns string,
) <-chan flow.Event[output.Watch] {
	out := make(chan flow.Event[output.Watch])

	go func() {
		defer close(out)

		acc := output.Watch{Subject: ns, Events: []apidto.ArrowRuntimeDTO{}}
		emit := func(ev flow.Event[output.Watch]) bool {
			select {
			case out <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}

		lastState := ""
		seen := map[int]string{}

		for evt := range events {
			acc.Events = append(acc.Events, evt)

			if !emitWatchEvent(evt, &lastState, seen, emit) {
				return
			}
		}

		final := acc
		emit(flow.Event[output.Watch]{Kind: flow.EventDone, Final: &final})
	}()

	return out
}

// emitWatchEvent reports one snapshot as a state transition plus any step
// transitions it carries, and returns false once the consumer has gone away.
func emitWatchEvent(
	evt apidto.ArrowRuntimeDTO,
	lastState *string,
	seen map[int]string,
	emit func(flow.Event[output.Watch]) bool,
) bool {
	if evt.State != *lastState {
		*lastState = evt.State

		if !emit(flow.Event[output.Watch]{
			Kind:  flow.EventStep,
			Name:  evt.State,
			State: component.StepDone,
		}) {
			return false
		}
	}

	if evt.ActiveRun == nil {
		return true
	}

	for _, s := range evt.ActiveRun.Steps {
		if seen[s.Index] == s.Status {
			continue
		}

		seen[s.Index] = s.Status

		if !emit(flow.Event[output.Watch]{
			Kind:  flow.EventStep,
			Name:  lifecycle.StepTitle(s),
			State: stepState(s.Status),
		}) {
			return false
		}
	}

	return true
}

// viewWatch renders the transitions seen so far, and closes with the reason
// the stream ended so a finished watch does not look like a hung one.
func viewWatch(ns string) func([]component.Step, *output.Watch, theme.Theme) string {
	return func(steps []component.Step, final *output.Watch, t theme.Theme) string {
		if final == nil {
			return component.Steps(steps, t)
		}

		return component.Steps(steps, t) + "\n" + component.Outcome(component.Result{
			OK:      true,
			Subject: "watch " + ns,
			Message: "stream closed",
		}, t)
	}
}
