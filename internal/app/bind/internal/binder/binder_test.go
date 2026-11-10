package binder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type agg struct{ N int }

func evt(name string) asynxModels.Event[agg] {
	return asynxModels.Event[agg]{EventName: name, AggregateID: "a", Version: 1}
}

func TestBinder_DispatchDurable_RoutesByPattern(t *testing.T) {
	testCases := []struct {
		name      string
		pattern   string
		eventName string
		wantCalls int
	}{
		{name: "topic glob matches", pattern: asynx.Topic("thing.added.*"), eventName: "thing.added.x", wantCalls: 1},
		{name: "exact name matches", pattern: "thingadded", eventName: "thingadded", wantCalls: 1},
		{name: "raw expression matches", pattern: "thing.added.*", eventName: "thing.added.x", wantCalls: 1},
		{name: "other action skipped", pattern: asynx.Topic("thing.added.*"), eventName: "thing.removed.x", wantCalls: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := New[agg]()
			calls := 0
			require.NoError(t, b.Durable(tc.pattern, func(context.Context, asynxModels.Event[agg]) error {
				calls++
				return nil
			}))
			b.Ready()

			require.NoError(t, b.DispatchDurable(context.Background(), evt(tc.eventName)))
			assert.Equal(t, tc.wantCalls, calls)
		})
	}
}

func TestBinder_DispatchDurable_WaitsForReady(t *testing.T) {
	b := New[agg]()
	ran := make(chan struct{})
	require.NoError(t, b.Durable("thing.*", func(context.Context, asynxModels.Event[agg]) error {
		close(ran)
		return nil
	}))

	done := make(chan error, 1)
	go func() { done <- b.DispatchDurable(context.Background(), evt("thing.x")) }()

	select {
	case <-ran:
		t.Fatal("handler ran before Ready")
	case <-time.After(50 * time.Millisecond):
	}

	b.Ready()
	b.Ready()
	require.NoError(t, <-done)
	<-ran
}

func TestBinder_DispatchDurable_CancelledWhileWaiting(t *testing.T) {
	b := New[agg]()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := b.DispatchDurable(ctx, evt("thing.x"))
	require.ErrorIs(t, err, context.Canceled)
}

func TestBinder_DispatchDurable_RunsEveryRouteAndJoinsErrors(t *testing.T) {
	b := New[agg]()
	errA := errors.New("a failed")
	errB := errors.New("b failed")
	ranLast := false
	require.NoError(t, b.Durable("thing.*", func(context.Context, asynxModels.Event[agg]) error { return errA }))
	require.NoError(t, b.Durable("thing.*", func(context.Context, asynxModels.Event[agg]) error { return errB }))
	require.NoError(t, b.Durable("thing.*", func(context.Context, asynxModels.Event[agg]) error {
		ranLast = true
		return nil
	}))
	b.Ready()

	err := b.DispatchDurable(context.Background(), evt("thing.x"))
	require.ErrorIs(t, err, errA)
	require.ErrorIs(t, err, errB)
	assert.True(t, ranLast)
}

func TestBinder_DispatchSync(t *testing.T) {
	boom := errors.New("boom")
	testCases := []struct {
		name      string
		eventName string
		fn        SyncFn[agg]
		wantErr   error
		wantCalls int
	}{
		{
			name: "matching route runs", eventName: "thing.added", wantCalls: 1,
			fn: func(context.Context, asynxModels.Tx, asynxModels.Event[agg]) error { return nil },
		},
		{
			name: "route error aborts", eventName: "thing.added", wantCalls: 1, wantErr: boom,
			fn: func(context.Context, asynxModels.Tx, asynxModels.Event[agg]) error { return boom },
		},
		{
			name: "other event skipped", eventName: "thing.removed", wantCalls: 0,
			fn: func(context.Context, asynxModels.Tx, asynxModels.Event[agg]) error { return boom },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := New[agg]()
			calls := 0
			require.NoError(t, b.Sync("thing.added", func(ctx context.Context, tx asynxModels.Tx, e asynxModels.Event[agg]) error {
				calls++
				return tc.fn(ctx, tx, e)
			}))

			err := b.DispatchSync(context.Background(), nil, evt(tc.eventName))
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			wantCalls := tc.wantCalls
			if tc.eventName != "thing.added" {
				wantCalls = 0
			}
			assert.Equal(t, wantCalls, calls)
		})
	}
}

func TestBinder_Register_RejectsBadExpression(t *testing.T) {
	b := New[agg]()
	require.Error(t, b.Durable("a(b", func(context.Context, asynxModels.Event[agg]) error { return nil }))
	require.Error(t, b.Sync("a(b", nil))
}
