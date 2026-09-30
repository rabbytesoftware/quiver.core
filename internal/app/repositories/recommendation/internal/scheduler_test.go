package recommendationinternal_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	recommendationinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/recommendation/internal"
)

const waitFor = 5 * time.Second

func receive(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(waitFor):
		t.Fatalf("timed out waiting for %s", what)
	}
}

type schedulerFixture struct {
	entered chan struct{}
	release chan struct{}
	ticks   chan time.Time
	runs    atomic.Int64
	due     atomic.Bool
	failing atomic.Bool
}

func newSchedulerFixture() *schedulerFixture {
	return &schedulerFixture{
		entered: make(chan struct{}, 16),
		release: make(chan struct{}),
		ticks:   make(chan time.Time),
	}
}

func (f *schedulerFixture) scheduler() recommendationinternal.Scheduler {
	return recommendationinternal.NewScheduler(recommendationinternal.SchedulerConfig{
		Run: func(ctx context.Context) error {
			f.runs.Add(1)
			f.entered <- struct{}{}
			select {
			case <-f.release:
			case <-ctx.Done():
			}
			if f.failing.Load() {
				return errors.New("boom")
			}
			return nil
		},
		Due:      func(context.Context) bool { return f.due.Load() },
		Interval: time.Hour,
		Ticker: func(time.Duration) (<-chan time.Time, func()) {
			return f.ticks, func() {}
		},
	})
}

func TestScheduler_Trigger_RunsOneRefreshAndJoinsTheRunningOne(t *testing.T) {
	f := newSchedulerFixture()
	s := f.scheduler()

	s.Trigger(context.Background())
	receive(t, f.entered, "the first run")
	s.Trigger(context.Background())
	s.Trigger(context.Background())

	assert.True(t, s.Running())
	close(f.release)
	require.NoError(t, s.Shutdown(context.Background()))
	assert.EqualValues(t, 1, f.runs.Load(), "a trigger during a run joins it")
	assert.False(t, s.Running())
}

func TestScheduler_Trigger_AfterTheRunFinishes_StartsANewOne(t *testing.T) {
	f := newSchedulerFixture()
	close(f.release)
	s := f.scheduler()

	s.Trigger(context.Background())
	receive(t, f.entered, "the first run")
	require.Eventually(t, func() bool { return !s.Running() }, waitFor, time.Millisecond)
	s.Trigger(context.Background())
	receive(t, f.entered, "the second run")

	require.NoError(t, s.Shutdown(context.Background()))
	assert.EqualValues(t, 2, f.runs.Load())
}

func TestScheduler_Trigger_OutlivesTheCallersContext(t *testing.T) {
	f := newSchedulerFixture()
	s := f.scheduler()
	ctx, cancel := context.WithCancel(context.Background())

	s.Trigger(ctx)
	receive(t, f.entered, "the run")
	cancel()

	assert.True(t, s.Running(), "a request that ended does not cancel the refresh it started")
	close(f.release)
	require.NoError(t, s.Shutdown(context.Background()))
}

func TestScheduler_Start_DueAtStart_RefreshesImmediately(t *testing.T) {
	f := newSchedulerFixture()
	f.due.Store(true)
	s := f.scheduler()

	s.Start(context.Background())
	receive(t, f.entered, "the stale-on-start run")

	close(f.release)
	require.NoError(t, s.Shutdown(context.Background()))
	assert.EqualValues(t, 1, f.runs.Load())
}

func TestScheduler_Start_NotDueAtStart_WaitsForTheInterval(t *testing.T) {
	f := newSchedulerFixture()
	close(f.release)
	s := f.scheduler()

	s.Start(context.Background())
	f.ticks <- time.Now()
	receive(t, f.entered, "the ticked run")

	require.NoError(t, s.Shutdown(context.Background()))
	assert.EqualValues(t, 1, f.runs.Load(), "nothing ran before the first tick")
}

func TestScheduler_Start_TickDuringARun_DoesNotStartASecond(t *testing.T) {
	f := newSchedulerFixture()
	s := f.scheduler()

	s.Start(context.Background())
	f.ticks <- time.Now()
	receive(t, f.entered, "the ticked run")
	f.ticks <- time.Now()
	f.ticks <- time.Now()

	close(f.release)
	require.NoError(t, s.Shutdown(context.Background()))
	assert.EqualValues(t, 1, f.runs.Load())
}

func TestScheduler_Start_ContextCancelled_StopsTheLoopAndTheRun(t *testing.T) {
	f := newSchedulerFixture()
	f.due.Store(true)
	s := f.scheduler()
	ctx, cancel := context.WithCancel(context.Background())

	s.Start(ctx)
	receive(t, f.entered, "the run")
	cancel()

	require.NoError(t, s.Shutdown(context.Background()))
	assert.False(t, s.Running())
}

func TestScheduler_Shutdown_CancelsARunInFlight(t *testing.T) {
	f := newSchedulerFixture()
	s := f.scheduler()
	s.Trigger(context.Background())
	receive(t, f.entered, "the run")

	require.NoError(t, s.Shutdown(context.Background()))

	assert.False(t, s.Running())
}

func TestScheduler_Shutdown_StopsALoopWhoseContextIsStillLive(t *testing.T) {
	f := newSchedulerFixture()
	s := f.scheduler()

	s.Start(context.Background())

	require.NoError(t, s.Shutdown(context.Background()))
}

func TestScheduler_Shutdown_IsIdempotent(t *testing.T) {
	s := newSchedulerFixture().scheduler()

	require.NoError(t, s.Shutdown(context.Background()))
	require.NoError(t, s.Shutdown(context.Background()))
}

func TestScheduler_AfterShutdown_NothingStarts(t *testing.T) {
	f := newSchedulerFixture()
	s := f.scheduler()
	require.NoError(t, s.Shutdown(context.Background()))

	s.Trigger(context.Background())
	s.Start(context.Background())

	assert.Zero(t, f.runs.Load())
	assert.False(t, s.Running())
}

func TestScheduler_Shutdown_ExpiredContext_ReturnsItsError(t *testing.T) {
	f := newSchedulerFixture()
	unstoppable := recommendationinternal.NewScheduler(recommendationinternal.SchedulerConfig{
		Run: func(context.Context) error {
			f.entered <- struct{}{}
			<-f.release
			return nil
		},
		Due:      func(context.Context) bool { return false },
		Interval: time.Hour,
	})
	unstoppable.Trigger(context.Background())
	receive(t, f.entered, "the run")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := unstoppable.Shutdown(ctx)

	require.ErrorIs(t, err, context.Canceled)
	close(f.release)
	require.NoError(t, unstoppable.Shutdown(context.Background()))
}

func TestScheduler_FailedRun_IsLoggedAndTheSchedulerKeepsWorking(t *testing.T) {
	f := newSchedulerFixture()
	f.failing.Store(true)
	close(f.release)
	s := f.scheduler()

	s.Trigger(context.Background())
	receive(t, f.entered, "the failing run")
	require.Eventually(t, func() bool { return !s.Running() }, waitFor, time.Millisecond)
	s.Trigger(context.Background())
	receive(t, f.entered, "the next run")

	require.NoError(t, s.Shutdown(context.Background()))
}

func TestScheduler_Start_UsesARealTickerWhenNoneIsInjected(t *testing.T) {
	var runs atomic.Int64
	ran := make(chan struct{}, 1)
	s := recommendationinternal.NewScheduler(recommendationinternal.SchedulerConfig{
		Run: func(context.Context) error {
			runs.Add(1)
			select {
			case ran <- struct{}{}:
			default:
			}
			return nil
		},
		Due:      func(context.Context) bool { return false },
		Interval: time.Millisecond,
	})

	s.Start(context.Background())
	receive(t, ran, "a real tick")

	require.NoError(t, s.Shutdown(context.Background()))
	assert.Positive(t, runs.Load())
}
