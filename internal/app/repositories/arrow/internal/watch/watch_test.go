package watch

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newWatch(interval time.Duration, sweep func(context.Context)) *watch {
	return New(interval, sweep).(*watch)
}

func noJitter(time.Duration) time.Duration { return 0 }

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		require.FailNow(t, what)
	}
}

func TestVersionWatch_SweepsAtStartThenEveryInterval(t *testing.T) {
	var sweeps atomic.Int32
	twice := make(chan struct{})
	w := newWatch(time.Millisecond, func(context.Context) {
		if sweeps.Add(1) == 2 {
			close(twice)
		}
	})
	w.jitter = noJitter

	w.Start(context.Background())
	waitClosed(t, twice, "the watch never swept twice")
	w.Stop()
	after := sweeps.Load()

	w.Stop()
	assert.Equal(t, after, sweeps.Load(), "a stopped watch sweeps no more")
}

func TestVersionWatch_StopWaitsForTheSweepInProgress(t *testing.T) {
	entered := make(chan struct{})
	finished := make(chan struct{})
	w := newWatch(time.Hour, func(ctx context.Context) {
		close(entered)
		<-ctx.Done()
		close(finished)
	})
	w.jitter = noJitter

	w.Start(context.Background())
	waitClosed(t, entered, "the first sweep never began")
	w.Stop()

	select {
	case <-finished:
	default:
		t.Fatal("stop returned while a sweep was still writing")
	}
}

func TestVersionWatch_EndsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := newWatch(time.Hour, func(context.Context) {})
	w.jitter = func(time.Duration) time.Duration { return time.Hour }

	w.Start(ctx)
	cancel()

	waitClosed(t, w.done, "the watch outlived its context")
}

func TestVersionWatch_OffOrNeverStarted_DoesNothing(t *testing.T) {
	var sweeps atomic.Int32
	w := newWatch(0, func(context.Context) { sweeps.Add(1) })

	w.Stop()
	w.Start(context.Background())
	w.Stop()

	assert.Nil(t, w.done, "an interval of zero starts no goroutine")
	assert.Zero(t, sweeps.Load())
}

func TestVersionWatch_StartTwice_RunsOneWatch(t *testing.T) {
	w := newWatch(time.Hour, func(context.Context) {})
	w.jitter = func(time.Duration) time.Duration { return time.Hour }

	w.Start(context.Background())
	first := w.done
	w.Start(context.Background())

	assert.Equal(t, first, w.done)
	w.Stop()
}

func TestRandomJitter_StaysBelowItsLimit(t *testing.T) {
	assert.Zero(t, randomJitter(0))
	assert.Zero(t, randomJitter(-time.Second))
	for range 100 {
		got := randomJitter(10 * time.Millisecond)
		assert.GreaterOrEqual(t, got, time.Duration(0))
		assert.Less(t, got, 10*time.Millisecond)
	}
}

func TestParseCheckInterval(t *testing.T) {
	testCases := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "an interval", raw: "3h", want: 3 * time.Hour},
		{name: "off", raw: "0s", want: 0},
		{name: "negative falls back", raw: "-1h", want: defaultVersionCheckInterval},
		{name: "unreadable falls back", raw: "often", want: defaultVersionCheckInterval},
		{name: "unset falls back", raw: "", want: defaultVersionCheckInterval},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseCheckInterval(tc.raw))
		})
	}
}

func TestResolveVersionCheckInterval_ReadsTheConfiguredValue(t *testing.T) {
	assert.GreaterOrEqual(t, resolveVersionCheckInterval(), time.Duration(0))
}

func TestAppOpts_CheckInterval_OptionWinsOverConfig(t *testing.T) {
	off := time.Duration(0)

	assert.Zero(t, Interval(&off))
	assert.Equal(t, resolveVersionCheckInterval(), Interval(nil))
}
