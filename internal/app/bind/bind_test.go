package bind

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	eventstore "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
)

type agg struct{ N int }

func evt(name string) asynxModels.Event[agg] {
	return asynxModels.Event[agg]{EventName: name, AggregateID: "a", Version: 1}
}

type fakeBound struct {
	asynx.Asynx[agg]
	durable []string
	sync    []string
	ready   bool
	err     error
}

func (f *fakeBound) Durable(pattern string, _ asynxModels.DurableHandler[agg]) error {
	f.durable = append(f.durable, pattern)
	return f.err
}

func (f *fakeBound) Sync(pattern string, _ SyncFn[agg]) error {
	f.sync = append(f.sync, pattern)
	return f.err
}

func (f *fakeBound) Ready() { f.ready = true }

func (f *fakeBound) Host() *gorm.DB { return nil }

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

func TestBound_RegistersRoutesAndHostsDB(t *testing.T) {
	f := &fakeBound{}

	require.NoError(t, OnEvent[agg](f, "thing.*", func(context.Context, asynxModels.Event[agg]) error { return nil }))
	ok, err := Projection[agg](f, "thing.*", func(context.Context, asynxModels.Tx, asynxModels.Event[agg]) error { return nil })
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, []string{"thing.*"}, f.durable)
	assert.Equal(t, []string{"thing.*"}, f.sync)
	assert.Nil(t, Host[agg](f))
	assert.Nil(t, Host[agg](&plainAsynx{}))
}

func TestProjection_PlainAsynx_ReportsFalse(t *testing.T) {
	ok, err := Projection[agg](&plainAsynx{}, "thing.*", nil)
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = Projection[agg](&fakeBound{err: errors.New("bad")}, "a(b", nil)
	require.Error(t, err)
}

func TestBuild_RegistersAfterwardsAndReleasesOnReady(t *testing.T) {
	dir := t.TempDir()
	es, err := eventstore.NewEventStore(filepath.Join(dir, "events.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = es.Close() })
	ss, err := eventstore.NewSnapshotStore(filepath.Join(dir, "snapshots.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	built, err := Build(asynx.New[agg]().WithEventStore(es).WithSnapshotStore(ss), es)
	require.NoError(t, err)
	t.Cleanup(func() { _ = built.Shutdown(context.Background()) })

	require.NoError(t, OnEvent[agg](built, "thing.*", func(context.Context, asynxModels.Event[agg]) error { return nil }))
	ok, err := Projection[agg](built, "thing.*", func(context.Context, asynxModels.Tx, asynxModels.Event[agg]) error { return nil })
	require.NoError(t, err)
	assert.True(t, ok)
	built.Ready()
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
		})
	}
}

func TestBuild_WithoutEventStore_Fails(t *testing.T) {
	built, err := Build(asynx.New[agg](), nil)
	require.Error(t, err)
	assert.Nil(t, built)
}
