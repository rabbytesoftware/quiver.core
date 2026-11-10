package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	eventstore "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	adaptersqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func hostedFixture(t *testing.T) (eventstore.Store, *gormdb.DB) {
	t.Helper()
	es, err := eventstore.NewEventStore(filepath.Join(t.TempDir(), "device.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = es.Close() })
	return es, es.(eventstore.Hosted).DB()
}

func device(id string) auth.Device {
	return auth.Device{
		ID:         id,
		Label:      "laptop",
		TokenHash:  "hash-" + id,
		State:      auth.DeviceStateActive,
		PairedAt:   time.Unix(100, 0).UTC(),
		LastSeenAt: time.Unix(200, 0).UTC(),
	}
}

func TestNewHosted_RequiresAHost(t *testing.T) {
	_, err := NewHosted(nil, nil)
	require.Error(t, err)
}

func TestHostedStore_UpsertTx_CommitsAndRollsBackWithTheTransaction(t *testing.T) {
	testCases := []struct {
		name string
		fail bool
	}{
		{name: "commit keeps the row"},
		{name: "rollback drops the row", fail: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			es, host := hostedFixture(t)
			st, err := NewHosted(host, nil)
			require.NoError(t, err)

			err = es.Atomically(context.Background(), func(tx asynxModels.Tx) error {
				if err := st.UpsertTx(context.Background(), tx, device("d1")); err != nil {
					return err
				}
				if tc.fail {
					return sql.ErrTxDone
				}
				return nil
			})

			_, getErr := st.Get(context.Background(), "d1")
			if tc.fail {
				require.ErrorIs(t, err, sql.ErrTxDone)
				require.Error(t, getErr)
				return
			}
			require.NoError(t, err)
			require.NoError(t, getErr)
		})
	}
}

func TestHostedStore_UpsertTx_Errors(t *testing.T) {
	es, host := hostedFixture(t)
	st, err := NewHosted(host, nil)
	require.NoError(t, err)

	t.Run("tx not backed by sql", func(t *testing.T) {
		err := st.UpsertTx(context.Background(), nonSQLTx{}, device("d1"))
		require.ErrorIs(t, err, eventstore.ErrNotSQLTx)
	})

	t.Run("write failure", func(t *testing.T) {
		err := es.Atomically(context.Background(), func(tx asynxModels.Tx) error {
			_, err := tx.Native().(*sql.Tx).ExecContext(context.Background(), "DROP TABLE auth_devices")
			require.NoError(t, err)
			return st.UpsertTx(context.Background(), tx, device("d1"))
		})
		require.Error(t, err)
	})
}

func TestNewHosted_ImportsLegacyRowsWithoutOverwriting(t *testing.T) {
	legacy, err := adaptersqlite.OpenDB(filepath.Join(t.TempDir(), "device_store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adaptersqlite.CloseDB(legacy) })
	old, err := New(legacy)
	require.NoError(t, err)
	require.NoError(t, old.Upsert(context.Background(), device("a")))
	stale := device("b")
	require.NoError(t, old.Upsert(context.Background(), stale))

	_, host := hostedFixture(t)
	first, err := NewHosted(host, nil)
	require.NoError(t, err)
	revoked := device("b")
	revoked.State = auth.DeviceStateRevoked
	require.NoError(t, first.Upsert(context.Background(), revoked))

	st, err := NewHosted(host, legacy)
	require.NoError(t, err)

	all, err := st.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 2)
	got, err := st.Get(context.Background(), "b")
	require.NoError(t, err)
	assert.Equal(t, auth.DeviceStateRevoked, got.State, "the newer row wins over the legacy one")
}

func TestNewHosted_LegacyWithoutTableImportsNothing(t *testing.T) {
	legacy, err := adaptersqlite.OpenDB(filepath.Join(t.TempDir(), "device_store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adaptersqlite.CloseDB(legacy) })

	_, host := hostedFixture(t)
	st, err := NewHosted(host, legacy)
	require.NoError(t, err)
	all, err := st.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, all)
}

type nonSQLTx struct{ asynxModels.Tx }

func (nonSQLTx) Native() any { return "not a connection" }
