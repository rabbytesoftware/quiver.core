// Package shutdown runs the ordered, bounded release of process resources.
//
// Every phase of a sequence runs under a deadline of its own, derived from
// context.Background(). Sharing one context across sibling phases is what lets a
// slow phase silently disable every phase after it: the first one to exhaust the
// budget hands its successors an already-dead context, and a drain handed a dead
// context returns immediately without persisting anything.
package shutdown

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// DiscardTimeout bounds the release of handles opened by a constructor that then
// failed. Nothing is in flight at construction time, so this is a guard against a
// wedged drain rather than a budget anything is expected to use.
const DiscardTimeout = 5 * time.Second

const (
	unbounded time.Duration = 0
	expired   time.Duration = -1
)

// Sequence runs every phase in order, each under its own Timeout, and returns
// every failure joined together, prefixed with prefix and the phase name.
//
// A failed phase never skips the ones after it: an aborted sequence would leave
// aggregates accepting writes or database handles open with their WAL unchecked.
func Sequence(
	prefix string,
	phases []Phase,
) error {
	return run(prefix, phases, func(p Phase) time.Duration { return p.Timeout })
}

// Split runs every phase in order under an equal share of the time left on ctx,
// each share derived from context.Background() so no phase can spend a sibling's.
// The shares add up to what ctx had left, so the sequence still finishes inside
// the caller's own deadline instead of overrunning it three times over.
//
// A ctx carrying no deadline leaves every phase unbounded. A ctx already done
// gives every phase a share that has already run out, so no phase starts and the
// sequence reports the expiry once per phase.
func Split(
	ctx context.Context,
	prefix string,
	phases []Phase,
) error {
	share := shareOf(ctx, len(phases))
	return run(prefix, phases, func(Phase) time.Duration { return share })
}

// CloseAll closes every closer, best effort. Used to release handles already
// opened when a later step of the same constructor fails.
func CloseAll(closers ...io.Closer) {
	for _, cl := range closers {
		_ = cl.Close()
	}
}

func run(
	prefix string,
	phases []Phase,
	budget func(p Phase) time.Duration,
) error {
	var errs []error

	for _, p := range phases {
		if err := runPhase(budget(p), p.Run); err != nil {
			errs = append(errs, fmt.Errorf("%s: %s: %w", prefix, p.Name, err))
		}
	}

	return errors.Join(errs...)
}

// runPhase enforces the deadline itself rather than trusting fn to observe it:
// handing a phase a context proves nothing about whether it reads one, and a
// single phase blocking on an unbounded wait would stall the whole sequence and
// stop the process from ever exiting. A phase that overruns loses its turn, not
// the shutdown.
func runPhase(
	timeout time.Duration,
	fn func(ctx context.Context) error,
) error {
	ctx, cancel := phaseContext(timeout)
	defer cancel()

	// A share that has already run out starts nothing. Phases are ordered for a
	// reason, and launching goroutines nobody waits for would run the whole
	// sequence concurrently instead.
	if err := ctx.Err(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() { done <- fn(ctx) }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func phaseContext(
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if timeout == unbounded {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), timeout)
}

func shareOf(
	ctx context.Context,
	phases int,
) time.Duration {
	if phases <= 0 {
		return unbounded
	}
	if ctx.Err() != nil {
		return expired
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		return unbounded
	}

	share := time.Until(deadline) / time.Duration(phases)
	if share <= 0 {
		return expired
	}
	return share
}
