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

// Wait consumes runtime events until the invoked method completes. onEvent,
// when non-nil, observes every event (for rendering). It errors when the
// stream ends without a terminal event or ctx expires.
func Wait(
	ctx context.Context,
	events <-chan apidto.ArrowRuntimeDTO,
	method string,
	onEvent func(apidto.ArrowRuntimeDTO),
) (Result, error) {
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
			if res, done := terminal(evt, method); done {
				return res, nil
			}
		}
	}
}

// terminal checks whether an event carries the invoked method's return.
func terminal(evt apidto.ArrowRuntimeDTO, method string) (Result, bool) {
	if evt.ActiveRun != nil || evt.LastReturn == nil {
		return Result{}, false
	}
	if !MatchesMethod(evt.LastReturn.Method, method) {
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

// UntitledStep is rendered when a manifest step has no title.
const UntitledStep = "[untitled step]"

// StepTitle returns the step's display title.
func StepTitle(s apidto.StepProgressDTO) string {
	if s.Title == "" {
		return UntitledStep
	}
	return s.Title
}
