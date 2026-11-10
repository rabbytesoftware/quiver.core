package bind

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventstore "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
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

type plainAsynx struct {
	asynx.Asynx[agg]
	pattern string
	handler asynxModels.ProjectionHandler[agg]
	err     error
}

func (p *plainAsynx) Subscribe(pattern string, h asynxModels.ProjectionHandler[agg], _ ...asynxModels.SubscriptionOpt[agg]) (string, error) {
	p.pattern, p.handler = pattern, h
	return "id", p.err
}

func TestOnEvent_PlainAsynx_FallsBackToSubscribe(t *testing.T) {
	testCases := []struct {
		name       string
		subscribe  error
		handlerErr error
		wantErr    bool
	}{
		{name: "subscribed"},
		{name: "handler error is only logged", handlerErr: errors.New("x")},
		{name: "subscribe error returned", subscribe: errors.New("no"), wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := &plainAsynx{err: tc.subscribe}
			called := false
			err := OnEvent[agg](p, "thing.*", func(context.Context, asynxModels.Event[agg]) error {
				called = true
				return tc.handlerErr
			})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "thing.*", p.pattern)
			p.handler(context.Background(), evt("thing.x"))
			assert.True(t, called)
		})
	}
}

func TestBound_RegistersOnBinderAndHostsDB(t *testing.T) {
	b := New[agg]()
	bound := Wrap[agg](nil, b, nil)

	require.NoError(t, OnEvent[agg](bound, "thing.*", func(context.Context, asynxModels.Event[agg]) error { return nil }))
	ok, err := Projection[agg](bound, "thing.*", func(context.Context, asynxModels.Tx, asynxModels.Event[agg]) error { return nil })
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Same(t, b, bound.Binder())
	assert.Nil(t, Host[agg](bound))
	assert.Nil(t, Host[agg](&plainAsynx{}))

	bound.Ready()
	select {
	case <-b.(*binder[agg]).ready:
	default:
		t.Fatal("Ready did not release the binder")
	}
}

func TestProjection_PlainAsynx_ReportsFalse(t *testing.T) {
	ok, err := Projection[agg](&plainAsynx{}, "thing.*", nil)
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = Projection[agg](Wrap[agg](nil, New[agg](), nil), "a(b", nil)
	require.Error(t, err)
}

func TestBinder_Register_RejectsBadExpression(t *testing.T) {
	b := New[agg]()
	require.Error(t, b.Durable("a(b", func(context.Context, asynxModels.Event[agg]) error { return nil }))
	require.Error(t, b.Sync("a(b", nil))
}

func TestBuild_HostsTheEventsDatabaseOnlyWhenTheStoreCanHostIt(t *testing.T) {
	testCases := []struct {
		name       string
		hostedPass bool
		wantHost   bool
	}{
		{name: "hosted store exposes its database", hostedPass: true, wantHost: true},
		{name: "unknown store hosts nothing", hostedPass: false, wantHost: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			es, err := eventstore.NewEventStore(filepath.Join(dir, "events.db"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = es.Close() })
			ss, err := eventstore.NewSnapshotStore(filepath.Join(dir, "snapshots.db"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = ss.Close() })

			var events asynxModels.Store
			if tc.hostedPass {
				events = es
			}
			built, err := Build(asynx.New[agg]().WithEventStore(es).WithSnapshotStore(ss), events)
			require.NoError(t, err)
			t.Cleanup(func() { _ = built.Shutdown(context.Background()) })

			assert.Equal(t, tc.wantHost, Host[agg](built) != nil)
			assert.NotNil(t, built.Binder())
		})
	}
}

func TestBuild_WithoutEventStore_Fails(t *testing.T) {
	built, err := Build(asynx.New[agg](), nil)
	require.Error(t, err)
	assert.Nil(t, built)
}
