package shutdown_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/shutdown"
)

const testTimeout = 5 * time.Second

// recordingPhases builds one phase per name, each appending itself to ran when
// invoked and returning the error registered for it.
func recordingPhases(
	ran *[]string,
	errs map[string]error,
	names ...string,
) []shutdown.Phase {
	phases := make([]shutdown.Phase, 0, len(names))
	for _, name := range names {
		phases = append(phases, shutdown.Phase{
			Name:    name,
			Timeout: testTimeout,
			Run: func(_ context.Context) error {
				*ran = append(*ran, name)
				return errs[name]
			},
		})
	}
	return phases
}

func TestSequence_RunsPhasesInOrder(t *testing.T) {
	var ran []string

	require.NoError(t, shutdown.Sequence("test", recordingPhases(&ran, nil, "first", "second", "third")))
	assert.Equal(t, []string{"first", "second", "third"}, ran)
}

func TestSequence_RunsEveryPhaseDespiteFailure(t *testing.T) {
	var ran []string
	firstErr := errors.New("first boom")

	err := shutdown.Sequence("test", recordingPhases(
		&ran,
		map[string]error{"first": firstErr},
		"first", "second", "third",
	))

	require.Error(t, err)
	assert.ErrorIs(t, err, firstErr)
	assert.Equal(t, []string{"first", "second", "third"}, ran,
		"a failed phase must not skip the ones after it")
}

func TestSequence_JoinsEveryPhaseError(t *testing.T) {
	var ran []string
	firstErr := errors.New("first boom")
	lastErr := errors.New("last boom")

	err := shutdown.Sequence("test", recordingPhases(
		&ran,
		map[string]error{"first": firstErr, "last": lastErr},
		"first", "middle", "last",
	))

	require.Error(t, err)
	assert.ErrorIs(t, err, firstErr)
	assert.ErrorIs(t, err, lastErr)
	assert.Contains(t, err.Error(), "test: first")
	assert.Contains(t, err.Error(), "test: last")
}

func TestSequence_GivesEveryPhaseItsOwnDeadline(t *testing.T) {
	var deadline time.Time
	var hasDeadline bool

	require.NoError(t, shutdown.Sequence("test", []shutdown.Phase{{
		Name:    "only",
		Timeout: testTimeout,
		Run: func(ctx context.Context) error {
			deadline, hasDeadline = ctx.Deadline()
			return nil
		},
	}}))

	require.True(t, hasDeadline, "a phase must run under a deadline of its own")
	assert.False(t, deadline.IsZero())
}

func TestSequence_PhaseIgnoresContext_StillReturns(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	// The phase blocks on a channel rather than on ctx, so only the sequence's own
	// bound can end it. Without that bound this call never returns.
	err := shutdown.Sequence("test", []shutdown.Phase{{
		Name:    "deaf",
		Timeout: time.Millisecond,
		Run: func(_ context.Context) error {
			<-release
			return nil
		},
	}})

	require.Error(t, err, "a phase that ignores its context must not stall the sequence")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSequence_ZeroTimeout_LeavesThePhaseUnbounded(t *testing.T) {
	var hasDeadline bool

	require.NoError(t, shutdown.Sequence("test", []shutdown.Phase{{
		Name: "unbounded",
		Run: func(ctx context.Context) error {
			_, hasDeadline = ctx.Deadline()
			return nil
		},
	}}))

	assert.False(t, hasDeadline, "a zero timeout must leave the phase without a deadline")
}

func TestSequence_ExpiredPhaseDoesNotStarveTheNext(t *testing.T) {
	var next error

	err := shutdown.Sequence("test", []shutdown.Phase{
		{
			Name:    "burns its budget",
			Timeout: time.Nanosecond,
			Run: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		},
		{
			Name:    "next",
			Timeout: testTimeout,
			Run: func(ctx context.Context) error {
				next = ctx.Err()
				return nil
			},
		},
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NoError(t, next, "a phase that spent its whole budget must not hand the next one a dead context")
}

func TestSplit_SlowPhaseDoesNotStarveTheNext(t *testing.T) {
	var next error
	var ran []string

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := shutdown.Split(ctx, "test", []shutdown.Phase{
		{
			Name: "burns its share",
			Run: func(phaseCtx context.Context) error {
				<-phaseCtx.Done()
				return phaseCtx.Err()
			},
		},
		{
			Name: "next",
			Run: func(phaseCtx context.Context) error {
				ran = append(ran, "next")
				next = phaseCtx.Err()
				return nil
			},
		},
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, []string{"next"}, ran, "the phase after the slow one must still run")
	assert.NoError(t, next, "each phase must get a share the previous one could not spend")
}

func TestSplit_SharesSumToTheCallerBudget(t *testing.T) {
	budget := time.Hour
	shares := make([]time.Duration, 0, 3)

	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	observe := func(phaseCtx context.Context) error {
		deadline, ok := phaseCtx.Deadline()
		require.True(t, ok)
		shares = append(shares, time.Until(deadline))
		return nil
	}

	require.NoError(t, shutdown.Split(ctx, "test", []shutdown.Phase{
		{Name: "one", Run: observe},
		{Name: "two", Run: observe},
		{Name: "three", Run: observe},
	}))

	require.Len(t, shares, 3)
	for _, share := range shares {
		assert.Less(t, share, budget/2,
			"three phases must split the caller budget instead of each taking all of it")
	}
}

func TestSplit_NoDeadline_LeavesEveryPhaseUnbounded(t *testing.T) {
	var hasDeadline bool

	require.NoError(t, shutdown.Split(context.Background(), "test", []shutdown.Phase{{
		Name: "only",
		Run: func(ctx context.Context) error {
			_, hasDeadline = ctx.Deadline()
			return nil
		},
	}}))

	assert.False(t, hasDeadline)
}

func TestSplit_CancelledCaller_ReportsEveryPhaseWithoutStartingAny(t *testing.T) {
	started := make(chan string, 2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := shutdown.Split(ctx, "test", []shutdown.Phase{
		{Name: "one", Run: func(_ context.Context) error { started <- "one"; return nil }},
		{Name: "two", Run: func(_ context.Context) error { started <- "two"; return nil }},
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "test: one")
	assert.Contains(t, err.Error(), "test: two",
		"an already-dead caller context must not stop the sequence from reporting every phase")
	assert.Empty(t, started, "a share that has already run out must not start its phase")
}

func TestSplit_ExhaustedCaller_ReportsEveryPhaseWithoutStartingAny(t *testing.T) {
	started := make(chan string, 1)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancel()

	err := shutdown.Split(ctx, "test", []shutdown.Phase{
		{Name: "one", Run: func(_ context.Context) error { started <- "one"; return nil }},
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Empty(t, started)
}

// pastDeadlineContext reports a deadline that has gone by while still reporting
// itself live, so the share works out to nothing without the context having been
// cancelled. A real context reaches this state for the instant between its
// deadline passing and its cancellation propagating.
type pastDeadlineContext struct{ context.Context }

func (c pastDeadlineContext) Deadline() (time.Time, bool) {
	return time.Now().Add(-time.Hour), true
}

func (c pastDeadlineContext) Err() error { return nil }

func TestSplit_ShareRoundsToNothing_ReportsEveryPhaseWithoutStartingAny(t *testing.T) {
	started := make(chan string, 1)

	err := shutdown.Split(pastDeadlineContext{context.Background()}, "test", []shutdown.Phase{
		{Name: "one", Run: func(_ context.Context) error { started <- "one"; return nil }},
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Empty(t, started, "a share with no time in it must not start its phase")
}

func TestSplit_NoPhases_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	assert.NoError(t, shutdown.Split(ctx, "test", nil))
}

type recordingCloser struct {
	closed bool
	err    error
}

func (c *recordingCloser) Close() error {
	c.closed = true
	return c.err
}

func TestCloseAll_ClosesEveryCloserDespiteFailure(t *testing.T) {
	first := &recordingCloser{err: errors.New("close boom")}
	second := &recordingCloser{}

	shutdown.CloseAll(first, second)

	assert.True(t, first.closed)
	assert.True(t, second.closed, "a failed close must not skip the closers after it")
}

func TestCloseAll_NoClosers_DoesNothing(t *testing.T) {
	assert.NotPanics(t, func() { shutdown.CloseAll() })
}

func TestDiscardTimeout_IsPositive(t *testing.T) {
	assert.Positive(t, shutdown.DiscardTimeout)
}
