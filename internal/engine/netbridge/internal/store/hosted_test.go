package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventstore "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
)

func hostedES(t *testing.T) eventstore.Store {
	t.Helper()
	es, err := eventstore.NewEventStore(filepath.Join(t.TempDir(), "netbridge.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = es.Close() })
	return es
}

func alloc(port int) ports.PortAllocation {
	return ports.PortAllocation{Port: port, Protocol: netbridge.ProtocolTCP, OwnerKey: "owner", Forwarded: true}
}

func TestHostedPortStore_Tx_CommitsAndRollsBack(t *testing.T) {
	testCases := []struct {
		name string
		fail bool
	}{
		{name: "commit keeps the writes"},
		{name: "rollback drops the writes", fail: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			es := hostedES(t)
			st, err := NewPortHosted(es.(eventstore.Hosted).DB(), "")
			require.NoError(t, err)
			require.NoError(t, st.Save(context.Background(), alloc(50001)))

			err = es.Atomically(context.Background(), func(tx asynxModels.Tx) error {
				if err := st.SaveTx(context.Background(), tx, alloc(50002)); err != nil {
					return err
				}
				if err := st.DeleteTx(context.Background(), tx, 50001); err != nil {
					return err
				}
				if tc.fail {
					return sql.ErrTxDone
				}
				return nil
			})

			kept, _ := st.FindByPort(context.Background(), 50001)
			added, _ := st.FindByPort(context.Background(), 50002)
			if tc.fail {
				require.ErrorIs(t, err, sql.ErrTxDone)
				assert.NotNil(t, kept)
				assert.Nil(t, added)
				return
			}
			require.NoError(t, err)
			assert.Nil(t, kept)
			assert.NotNil(t, added)
		})
	}
}

func TestHostedPortStore_Tx_Errors(t *testing.T) {
	es := hostedES(t)
	st, err := NewPortHosted(es.(eventstore.Hosted).DB(), "")
	require.NoError(t, err)

	t.Run("tx not backed by sql", func(t *testing.T) {
		require.ErrorIs(t, st.SaveTx(context.Background(), nonSQLTx{}, alloc(1)), eventstore.ErrNotSQLTx)
		require.ErrorIs(t, st.DeleteTx(context.Background(), nonSQLTx{}, 1), eventstore.ErrNotSQLTx)
	})

	t.Run("write failure", func(t *testing.T) {
		err := es.Atomically(context.Background(), func(tx asynxModels.Tx) error {
			_, err := tx.Native().(*sql.Tx).ExecContext(context.Background(), "DROP TABLE port_allocations")
			require.NoError(t, err)
			require.Error(t, st.DeleteTx(context.Background(), tx, 1))
			return st.SaveTx(context.Background(), tx, alloc(1))
		})
		require.Error(t, err)
	})
}

func TestNewPortHosted_ImportsTheLegacyFileOnce(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "netbridge_ports.db")
	legacy, err := NewPortSQLite(legacyPath)
	require.NoError(t, err)
	require.NoError(t, legacy.Save(context.Background(), alloc(50010)))
	require.NoError(t, legacy.Save(context.Background(), alloc(50011)))
	require.NoError(t, legacy.Close())

	es := hostedES(t)
	st, err := NewPortHosted(es.(eventstore.Hosted).DB(), legacyPath)
	require.NoError(t, err)

	all, err := st.FindAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 2)
	_, err = os.Stat(legacyPath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(legacyPath + ".migrated")
	assert.NoError(t, err)

	require.NoError(t, st.Close())
	again, err := NewPortHosted(es.(eventstore.Hosted).DB(), legacyPath)
	require.NoError(t, err)
	all, err = again.FindAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 2, "the host stays open after Close and a repeat adds nothing")
}

func TestNewPortHosted_LegacyEdgeCases(t *testing.T) {
	t.Run("file without the table imports nothing", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "empty.db")
		require.NoError(t, os.WriteFile(p, nil, 0o600))
		st, err := NewPortHosted(hostedES(t).(eventstore.Hosted).DB(), p)
		require.NoError(t, err)
		all, err := st.FindAll(context.Background())
		require.NoError(t, err)
		assert.Empty(t, all)
	})

	t.Run("garbage file fails", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "garbage.db")
		require.NoError(t, os.WriteFile(p, []byte("this is not a sqlite database at all, just text"), 0o600))
		_, err := NewPortHosted(hostedES(t).(eventstore.Hosted).DB(), p)
		require.Error(t, err)
	})

	t.Run("unstatable path fails", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("windows reports a path under a file as not found")
		}
		blocker := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(blocker, nil, 0o600))
		_, err := NewPortHosted(hostedES(t).(eventstore.Hosted).DB(), filepath.Join(blocker, "x.db"))
		require.Error(t, err)
	})
}

type nonSQLTx struct{ asynxModels.Tx }

func (nonSQLTx) Native() any { return "not a connection" }
