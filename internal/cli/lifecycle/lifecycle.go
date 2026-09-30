// Package lifecycle decides when a runtime method has finished.
//
// It consumes the runtime WebSocket event stream and reports the terminal
// result. It does no rendering: the CLI draws every command through
// internal/cli/tui, and a second renderer here is what let install and list
// drift into two different looks.
package lifecycle

import (
	"context"
	"fmt"
	"strings"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

// Result is the terminal outcome of one method execution.
type Result struct {
	Outcome    string // success | failed | cancelled
	State      string // arrow state after the run
	FailedStep *apidto.StepProgressDTO
	Steps      []apidto.StepProgressDTO
}

// MatchesMethod reports whether a runtime-recorded method name refers to the
// CLI-invoked method. The daemon records built-ins with an underscore prefix
// and the CLI's "run" invokes the manifest's "execute".
func MatchesMethod(recorded, invoked string) bool {
	r := strings.TrimPrefix(recorded, "_")
	i := strings.TrimPrefix(invoked, "_")
	if i == "run" {
		i = "execute"
	}
	return strings.EqualFold(r, i)
}

// Wait consumes runtime events until the invoked method completes. previous
// is the arrow's last return read before the method was invoked, nil when it
// had none or it could not be read: a return that is still that one belongs
// to an earlier run, even of the same method, and is never this run's end.
// onEvent, when non-nil, observes every event (for rendering). It errors when
// the stream ends without a terminal event or ctx expires.
func Wait(
	ctx context.Context,
	events <-chan apidto.ArrowRuntimeDTO,
	method string,
	previous *apidto.ReturnDTO,
	onEvent func(apidto.ArrowRuntimeDTO),
) (Result, error) {
	w := waiter{method: method, previous: previous}
	for {
		select {
		case <-ctx.Done():
			return Result{}, fmt.Errorf("lifecycle: waiting for %s: %w", method, ctx.Err())
		case evt, open := <-events:
			if !open {
				return Result{}, fmt.Errorf("lifecycle: event stream closed before %s completed", method)
			}
			if onEvent != nil {
				onEvent(evt)
			}
			if res, done := w.terminal(evt); done {
				return res, nil
			}
		}
	}
}

type waiter struct {
	method   string
	previous *apidto.ReturnDTO
	// began records an event showing a run of method active. It tells this
	// run's return apart from the previous one on a daemon whose returns
	// carry no execution ID.
	began bool
}

// terminal checks whether an event carries this run's return.
func (w *waiter) terminal(evt apidto.ArrowRuntimeDTO) (Result, bool) {
	if evt.ActiveRun != nil {
		w.began = w.began || MatchesMethod(evt.ActiveRun.Method, w.method)
		return Result{}, false
	}
	if evt.LastReturn == nil || !MatchesMethod(evt.LastReturn.Method, w.method) {
		return Result{}, false
	}
	if !w.isNew(evt.LastReturn) {
		return Result{}, false
	}

	res := Result{
		Outcome: evt.LastReturn.Outcome,
		State:   evt.State,
		Steps:   evt.LastReturn.Steps,
	}
	for i := range evt.LastReturn.Steps {
		if evt.LastReturn.Steps[i].Status == "failed" {
			res.FailedStep = &evt.LastReturn.Steps[i]
			break
		}
	}
	return res, true
}

func (w *waiter) isNew(ret *apidto.ReturnDTO) bool {
	if w.previous == nil {
		return true
	}
	if ret.ExecutionID != w.previous.ExecutionID {
		return true
	}
	return ret.ExecutionID == "" && w.began
}

// UntitledStep is rendered when a manifest step has no title.
const UntitledStep = "[untitled step]"

// StepTitle returns the step's display title.
func StepTitle(s apidto.StepProgressDTO) string {
	if s.Title == "" {
		return UntitledStep
	}
	return s.Title
}
