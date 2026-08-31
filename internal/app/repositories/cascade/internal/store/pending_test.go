package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/cascade/internal/store"
)

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	s, err := store.New(db)
	require.NoError(t, err)
	return s
}

func TestNew_NilDB(t *testing.T) {
	_, err := store.New(nil)
	require.Error(t, err)
}

func TestEnqueue_AndPending_Roundtrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Enqueue(ctx, "github.com/user/pkg@v1.0.0"))
	require.NoError(t, s.Enqueue(ctx, "github.com/user/other@v1.0.0"))

	pending, err := s.Pending(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"github.com/user/pkg@v1.0.0", "github.com/user/other@v1.0.0"}, pending)
}

func TestEnqueue_SameNamespaceTwice_IsNotAnError(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Enqueue(ctx, "github.com/user/pkg@v1.0.0"))
	require.NoError(t, s.Enqueue(ctx, "github.com/user/pkg@v1.0.0"))

	pending, err := s.Pending(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 1, "a duplicate enqueue must not create a second row")
}

func TestComplete_RemovesTheRow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.Enqueue(ctx, "github.com/user/pkg@v1.0.0"))
	require.NoError(t, s.Complete(ctx, "github.com/user/pkg@v1.0.0"))

	pending, err := s.Pending(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestComplete_UnknownNamespace_IsANoop(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.Complete(context.Background(), "github.com/user/never-enqueued@v1.0.0"))
}

func TestPending_Empty_ReturnsEmptySlice(t *testing.T) {
	s := newTestStore(t)
	pending, err := s.Pending(context.Background())
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestNew_ClosedDB_MigrateFails(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, adapterSQLite.CloseDB(db))

	_, err = store.New(db)
	require.Error(t, err)
}

func TestPending_ClosedDB_ReturnsError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	s, err := store.New(db)
	require.NoError(t, err)
	require.NoError(t, adapterSQLite.CloseDB(db))

	_, err = s.Pending(context.Background())
	require.Error(t, err)
}
