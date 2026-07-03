// Package lifecycle drives a runtime method to completion: it consumes the
// runtime WebSocket event stream, decides when the operation reached its
// terminal state, and renders step progress (plain line-per-transition when
// piped; the BubbleTea view wraps the same primitives on a TTY).
package lifecycle

import (
	"context"
	"fmt"
	"io"
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

// PlainPrinter prints one line per step transition — the non-TTY renderer.
type PlainPrinter struct {
	w    io.Writer
	seen map[int]string
}

// NewPlainPrinter builds a PlainPrinter writing to w.
func NewPlainPrinter(w io.Writer) *PlainPrinter {
	return &PlainPrinter{w: w, seen: map[int]string{}}
}

// Observe prints transitions found in one runtime event.
func (p *PlainPrinter) Observe(evt apidto.ArrowRuntimeDTO) {
	if evt.ActiveRun == nil {
		return
	}
	total := len(evt.ActiveRun.Steps)
	for _, s := range evt.ActiveRun.Steps {
		if s.Status == "pending" || p.seen[s.Index] == s.Status {
			continue
		}
		p.seen[s.Index] = s.Status
		fmt.Fprintf(p.w, "step %d/%d %s: %s\n", s.Index+1, total, s.Status, StepTitle(s))
		if s.Status == "failed" && s.Error != nil {
			fmt.Fprintf(p.w, "  %s\n", *s.Error)
		}
	}
}
