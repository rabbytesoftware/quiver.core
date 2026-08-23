package commands

import (
	"context"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/lifecycle"
	"github.com/rabbytesoftware/quiver.core/internal/cli/output"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/component"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/flow"
	"github.com/rabbytesoftware/quiver.core/internal/cli/tui/theme"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// streamRun drives one method to completion and renders its progress.
//
// The subscription opens before the method is fired so no step event can be
// missed in the gap between the two.
func (a *app) streamRun(cmd *cobra.Command, ns, op string, vars map[string]string) error {
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

	started, err := cli.ExecuteMethod(ctx, ns, apiMethod(op), vars)
	if err != nil {
		return err
	}

	if !started {
		// The daemon completed the request as an idempotent no-op. No runtime
		// events will arrive, so waiting for one would hang forever.
		cancel()

		return a.renderNoOp(cmd, ns, op)
	}

	model := flow.NewStreaming(runner.Theme(), flow.StreamOpts[output.Run]{
		Label: op + " " + ns,
		Start: func() (<-chan flow.Event[output.Run], error) {
			return translateRun(ctx, events, ns, op), nil
		},
		View: viewRun(ns, op),
	})

	return runner.Run(ctx, model)
}

// translateRun converts the daemon's runtime events into the flow's events.
//
// The client speaks in ArrowRuntimeDTO snapshots — each one carries the whole
// step list — while the flow wants one event per transition. Holding the last
// seen status per step index is what turns the former into the latter.
func translateRun(
	ctx context.Context,
	events <-chan apidto.ArrowRuntimeDTO,
	ns, op string,
) <-chan flow.Event[output.Run] {
	out := make(chan flow.Event[output.Run])

	go func() {
		defer close(out)

		seen := map[int]string{}

		res, err := lifecycle.Wait(ctx, events, op, func(evt apidto.ArrowRuntimeDTO) {
			if evt.ActiveRun == nil {
				return
			}

			for _, s := range evt.ActiveRun.Steps {
				if seen[s.Index] == s.Status {
					continue
				}

				seen[s.Index] = s.Status

				select {
				case out <- flow.Event[output.Run]{
					Kind:  flow.EventStep,
					Name:  lifecycle.StepTitle(s),
					State: stepState(s.Status),
				}:
				case <-ctx.Done():
					return
				}
			}
		})
		if err != nil {
			out <- flow.Event[output.Run]{Kind: flow.EventFailed, Err: err}

			return
		}

		run := runFrom(res, ns, op)
		if !run.OK() {
			out <- flow.Event[output.Run]{
				Kind: flow.EventFailed,
				Err:  runError{run: run},
			}

			return
		}

		out <- flow.Event[output.Run]{Kind: flow.EventDone, Final: &run}
	}()

	return out
}

func runFrom(res lifecycle.Result, ns, op string) output.Run {
	run := output.Run{
		Subject: ns,
		Method:  op,
		Outcome: res.Outcome,
		State:   res.State,
		Steps:   res.Steps,
		At:      time.Now().UTC().Format(time.RFC3339),
	}

	if run.Steps == nil {
		run.Steps = []apidto.StepProgressDTO{}
	}

	if res.FailedStep != nil && res.FailedStep.Error != nil {
		run.Error = *res.FailedStep.Error
	}

	return run
}

func stepState(status string) component.StepState {
	switch status {
	case "running":
		return component.StepRunning
	case "success", "succeeded", "done":
		return component.StepDone
	case "failed":
		return component.StepFailed
	}

	return component.StepPending
}

// viewRun renders the steps while the method runs and the outcome once it
// finishes. The subject is carried in the closure because the steps arrive
// before any payload does.
func viewRun(ns, op string) func([]component.Step, *output.Run, theme.Theme) string {
	return func(steps []component.Step, final *output.Run, t theme.Theme) string {
		if final == nil {
			return component.Steps(steps, t)
		}

		out := component.Steps(steps, t) + "\n" + component.Outcome(component.Result{
			OK:      final.OK(),
			Subject: op + " " + ns,
			Message: t.State(domain.ArrowState(final.State)),
		}, t)

		return out
	}
}

func (a *app) renderNoOp(cmd *cobra.Command, ns, op string) error {
	return renderInstant(
		a, cmd, "",
		func() (output.NoOp, error) {
			return output.NoOp{
				Subject: ns,
				Method:  op,
				Reason:  noOpDetail(op),
				At:      time.Now().UTC().Format(time.RFC3339),
			}, nil
		},
		func(n output.NoOp, t theme.Theme) string {
			return component.Outcome(component.Result{
				OK:      true,
				Subject: n.Subject,
				Message: n.Reason,
			}, t)
		},
	)
}

// runError is the error a failed lifecycle method reports.
//
// The daemon's own verdict is one word — "failed" — which tells a user
// nothing they could act on. Everything needed to say why is already in the
// run: which step stopped, what it was called, and what it said. This type
// exists to put those in the message rather than leave them in a payload the
// table path never prints.
type runError struct {
	run output.Run
}

func (e runError) Error() string {
	msg := e.run.Method + " " + e.run.Subject + ": " + e.run.Outcome

	if step, ok := e.failedStep(); ok {
		msg += " at step " + strconv.Itoa(step.Index+1) +
			"/" + strconv.Itoa(len(e.run.Steps)) +
			" " + strconv.Quote(lifecycle.StepTitle(step))
	}

	if e.run.Error != "" {
		msg += ": " + e.run.Error
	}

	// The arrow's resulting state is what decides the user's next move — a
	// failed install leaving "absent" is retryable, one leaving "installing"
	// is stuck and needs a reset.
	if e.run.State != "" {
		msg += " (state " + e.run.State + ")"
	}

	return msg
}

// Run exposes the payload so a caller can report the run in full.
func (e runError) Run() output.Run { return e.run }

func (e runError) failedStep() (apidto.StepProgressDTO, bool) {
	for _, s := range e.run.Steps {
		if s.Status == "failed" {
			return s, true
		}
	}

	return apidto.StepProgressDTO{}, false
}
