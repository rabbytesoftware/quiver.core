package events

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTxTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestEventStore_Tx_FailsWhenItsTableIsGone(t *testing.T) {
	testCases := []struct {
		name  string
		table string
		call  func(ctx context.Context, tx models.Tx) error
	}{
		{"append", "events", func(ctx context.Context, tx models.Tx) error { return tx.Append(ctx, "events:a", 1, []byte("x")) }},
		{"delete stream", "events", func(ctx context.Context, tx models.Tx) error { return tx.DeleteStream(ctx, "events:a") }},
		{"drop deliveries", "deliveries", func(ctx context.Context, tx models.Tx) error { return tx.DropDeliveries(ctx, "a") }},
		{"enqueue delivery", "deliveries", func(ctx context.Context, tx models.Tx) error { return tx.EnqueueDelivery(ctx, "a", 1) }},
		{"put receipt", "receipts", func(ctx context.Context, tx models.Tx) error { return tx.PutReceipt(ctx, "c", models.Receipt{}) }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTxTestStore(t)
			ctx := context.Background()

			err := s.Atomically(ctx, func(tx models.Tx) error {
				native, ok := tx.Native().(*sql.Tx)
				require.True(t, ok)
				_, execErr := native.ExecContext(ctx, "DROP TABLE "+tc.table)
				require.NoError(t, execErr)
				return tc.call(ctx, tx)
			})

			require.Error(t, err)
			assert.NotErrorIs(t, err, models.ErrPipelineFailed)
			assert.NotErrorIs(t, err, models.ErrDuplicateReceipt)
		})
	}
}

func TestEventStore_ClosedStore_ReadsAndWritesFail(t *testing.T) {
	s := newTxTestStore(t)
	require.NoError(t, s.Close())
	ctx := context.Background()

	_, _, err := s.GetReceipt(ctx, "c")
	assert.Error(t, err)

	_, err = s.PendingDeliveries(ctx, 10)
	assert.Error(t, err)

	assert.Error(t, s.AckDelivery(ctx, "a", 1))
	assert.Error(t, s.Atomically(ctx, func(models.Tx) error { return nil }))
}

func TestEventStore_Tx_EnqueueDeliveryTwiceKeepsOneEntry(t *testing.T) {
	s := newTxTestStore(t)
	ctx := context.Background()

	for range 2 {
		require.NoError(t, s.Atomically(ctx, func(tx models.Tx) error {
			return tx.EnqueueDelivery(ctx, "a", 1)
		}))
	}

	got, err := s.PendingDeliveries(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, []models.Delivery{{AggregateID: "a", Version: 1}}, got)
}

func TestEventStore_Tx_RolledBackWritesLeaveNothing(t *testing.T) {
	s := newTxTestStore(t)
	ctx := context.Background()

	err := s.Atomically(ctx, func(tx models.Tx) error {
		require.NoError(t, tx.Append(ctx, "events:a", 1, []byte("x")))
		require.NoError(t, tx.PutReceipt(ctx, "c", models.Receipt{Accepted: true}))
		require.NoError(t, tx.EnqueueDelivery(ctx, "a", 1))
		return assert.AnError
	})
	require.ErrorIs(t, err, assert.AnError)

	blobs, err := s.ReadFrom(ctx, "events:a", 0)
	require.NoError(t, err)
	assert.Empty(t, blobs)
	_, found, err := s.GetReceipt(ctx, "c")
	require.NoError(t, err)
	assert.False(t, found)
	pending, err := s.PendingDeliveries(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, pending)
}
