package cascade_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/cascade"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// closeDB closes the underlying sqlite connection so any later GORM call
// through db returns an error — used to force the store-error branches inside
// Cascade without a fake Store.
func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, adapterSQLite.CloseDB(db))
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	return db
}

func TestNew_NilDB(t *testing.T) {
	_, err := cascade.New(nil, func(context.Context, domain.Namespace) error { return nil })
	require.Error(t, err)
}

func TestEnqueue_ReturnsBeforeCascadeCompletes(t *testing.T) {
	db := newTestDB(t)
	release := make(chan struct{})
	forgetCalled := make(chan domain.Namespace, 1)

	c, err := cascade.New(db, func(_ context.Context, ns domain.Namespace) error {
		forgetCalled <- ns
		<-release
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { close(release) })

	ns := domain.Namespace("github.com/user/pkg@v1.0.0")

	done := make(chan error, 1)
	go func() { done <- c.Enqueue(context.Background(), ns) }()

	select {
	case enqueueErr := <-done:
		require.NoError(t, enqueueErr)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Enqueue must return without waiting for the cascade to finish")
	}

	select {
	case got := <-forgetCalled:
		assert.Equal(t, ns, got, "the background drain must still run ForgetRuntimeFn")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the background drain to call ForgetRuntimeFn")
	}
}

func TestEnqueue_Success_ClearsThePendingRow(t *testing.T) {
	db := newTestDB(t)
	forgotten := make(chan domain.Namespace, 1)

	c, err := cascade.New(db, func(_ context.Context, ns domain.Namespace) error {
		forgotten <- ns
		return nil
	})
	require.NoError(t, err)

	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	require.NoError(t, c.Enqueue(context.Background(), ns))

	select {
	case <-forgotten:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the background drain")
	}

	require.NoError(t, c.Shutdown(context.Background()))
	require.NoError(t, c.Drain(context.Background()),
		"a second drain over an already-completed row must be a no-op")
}

func TestDrain_RetriesAPendingRowUntilItSucceeds(t *testing.T) {
	db := newTestDB(t)
	var calls atomic.Int32
	var mu sync.Mutex
	failNext := true

	c, err := cascade.New(db, func(_ context.Context, _ domain.Namespace) error {
		calls.Add(1)
		mu.Lock()
		defer mu.Unlock()
		if failNext {
			failNext = false
			return errors.New("forget boom")
		}
		return nil
	})
	require.NoError(t, err)

	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	require.NoError(t, c.Enqueue(context.Background(), ns))

	require.Eventually(t, func() bool { return calls.Load() >= 1 }, 2*time.Second, 10*time.Millisecond,
		"the background drain kicked by Enqueue must attempt the cascade at least once")

	// The first attempt failed, so the row is still pending — Drain retries it.
	require.NoError(t, c.Drain(context.Background()))
	assert.GreaterOrEqual(t, int(calls.Load()), 2)
}

func TestDrain_OneFailureDoesNotBlockTheRest(t *testing.T) {
	db := newTestDB(t)
	var mu sync.Mutex
	var succeeded []domain.Namespace
	failNS := domain.Namespace("github.com/user/bad@v1.0.0")
	okNS := domain.Namespace("github.com/user/good@v1.0.0")

	forgetFn := func(_ context.Context, ns domain.Namespace) error {
		if ns == failNS {
			return errors.New("forget boom")
		}
		mu.Lock()
		succeeded = append(succeeded, ns)
		mu.Unlock()
		return nil
	}

	c, err := cascade.New(db, forgetFn)
	require.NoError(t, err)
	require.NoError(t, c.Enqueue(context.Background(), failNS))
	require.NoError(t, c.Enqueue(context.Background(), okNS))
	require.NoError(t, c.Shutdown(context.Background())) // wait out the auto-drain before asserting

	err = c.Drain(context.Background())
	require.Error(t, err, "a row that keeps failing must be reported")
	assert.Contains(t, succeeded, okNS, "one namespace failing must not stop the others from being drained")
}

func TestEnqueue_AfterShutdown_StillPersistsButDoesNotAutoDrain(t *testing.T) {
	db := newTestDB(t)
	var called atomic.Bool

	c, err := cascade.New(db, func(context.Context, domain.Namespace) error {
		called.Store(true)
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, c.Shutdown(context.Background()))

	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	require.NoError(t, c.Enqueue(context.Background(), ns), "Enqueue must still persist durably after Shutdown")

	time.Sleep(50 * time.Millisecond)
	assert.False(t, called.Load(), "Shutdown must stop new background drains, not just wait for old ones")

	require.NoError(t, c.Drain(context.Background()))
	assert.True(t, called.Load(), "an explicit Drain must still pick up a row enqueued after Shutdown")
}

func TestEnqueue_StoreError_Wrapped(t *testing.T) {
	db := newTestDB(t)
	c, err := cascade.New(db, func(context.Context, domain.Namespace) error { return nil })
	require.NoError(t, err)
	closeDB(t, db)

	err = c.Enqueue(context.Background(), domain.Namespace("github.com/user/pkg@v1.0.0"))
	require.Error(t, err)
}

func TestDrain_StorePendingError_Wrapped(t *testing.T) {
	db := newTestDB(t)
	c, err := cascade.New(db, func(context.Context, domain.Namespace) error { return nil })
	require.NoError(t, err)
	closeDB(t, db)

	err = c.Drain(context.Background())
	require.Error(t, err)
}

func TestRun_StoreCompleteError_Wrapped(t *testing.T) {
	db := newTestDB(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")

	// forgetRuntime succeeds but breaks the store as a side effect, so the
	// Complete call right after it fails — the only way to reach run's second
	// error branch without a fake Store.
	c, err := cascade.New(db, func(context.Context, domain.Namespace) error {
		closeDB(t, db)
		return nil
	})
	require.NoError(t, err)

	// Shutdown before Enqueue disables the auto-triggered background drain, so
	// this test can call Drain itself and inspect the returned error directly.
	require.NoError(t, c.Shutdown(context.Background()))
	require.NoError(t, c.Enqueue(context.Background(), ns))

	err = c.Drain(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "complete")
}

func TestShutdown_BoundedByContext(t *testing.T) {
	db := newTestDB(t)
	release := make(chan struct{})

	c, err := cascade.New(db, func(_ context.Context, _ domain.Namespace) error {
		<-release
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { close(release) })

	require.NoError(t, c.Enqueue(context.Background(), domain.Namespace("github.com/user/pkg@v1.0.0")))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = c.Shutdown(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
