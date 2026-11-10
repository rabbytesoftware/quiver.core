package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite/internal/entries"
)

func TestNewEventStore_OpensAHostedStore(t *testing.T) {
	s, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	hosted, ok := s.(Hosted)
	require.True(t, ok)
	assert.NotNil(t, hosted.DB())
}

func TestNewEventStore_InvalidPath_ReturnsAnUntypedNil(t *testing.T) {
	s, err := NewEventStore(filepath.Join(t.TempDir(), "no-such-dir", "events.db"))
	require.Error(t, err)
	assert.True(t, s == nil)
}

func TestNewSnapshotStore_OpensAStore(t *testing.T) {
	s, err := NewSnapshotStore(filepath.Join(t.TempDir(), "snapshots.db"))
	require.NoError(t, err)
	require.NoError(t, s.Close())
}

func TestNewSnapshotStore_InvalidPath_ReturnsAnUntypedNil(t *testing.T) {
	s, err := NewSnapshotStore(filepath.Join(t.TempDir(), "no-such-dir", "snapshots.db"))
	require.Error(t, err)
	assert.True(t, s == nil)
}

func TestErrVersionConflict_IsTheSentinelTheStoreReports(t *testing.T) {
	assert.Same(t, entries.ErrVersionConflict, ErrVersionConflict)
}

func TestTxSession(t *testing.T) {
	testCases := []struct {
		name string
		tx   func(db Hosted, ctx context.Context) models.Tx
		fail bool
	}{
		{name: "inside the event transaction the writes roll back with it"},
		{name: "a transaction that is not database/sql is refused", fail: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := NewEventStore(filepath.Join(t.TempDir(), "events.db"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = s.Close() })
			db := s.(Hosted).DB()
			ctx := context.Background()

			if tc.fail {
				_, err = TxSession(ctx, db, foreignTx{})
				require.ErrorIs(t, err, ErrNotSQLTx)
				return
			}

			boom := assert.AnError
			err = s.Atomically(ctx, func(tx models.Tx) error {
				session, sessErr := TxSession(ctx, db, tx)
				require.NoError(t, sessErr)
				require.NoError(t, session.Exec("CREATE TABLE scratch (id INTEGER)").Error)
				require.NoError(t, session.Exec("INSERT INTO scratch VALUES (1)").Error)
				return boom
			})
			require.ErrorIs(t, err, boom)

			var tables int
			require.NoError(t, db.Raw("SELECT count(*) FROM sqlite_master WHERE name = 'scratch'").Scan(&tables).Error)
			assert.Zero(t, tables)
		})
	}
}

type foreignTx struct{ models.Tx }

func (foreignTx) Native() any { return "not a pool" }
