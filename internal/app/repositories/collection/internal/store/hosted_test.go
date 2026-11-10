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
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func openHost(t *testing.T) eventstore.Store {
	t.Helper()
	es, err := eventstore.NewEventStore(filepath.Join(t.TempDir(), "collection.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = es.Close() })
	return es
}

func hostDB(t *testing.T, es eventstore.Store) eventstore.Hosted {
	t.Helper()
	h, ok := es.(eventstore.Hosted)
	require.True(t, ok)
	return h
}

func TestHostedStore_SaveTx_CommitsAndRollsBackWithTheTransaction(t *testing.T) {
	testCases := []struct {
		name      string
		failAfter bool
		wantRow   bool
	}{
		{name: "commit keeps the row", wantRow: true},
		{name: "rollback drops the row", failAfter: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			es := openHost(t)
			s, err := NewHosted(hostDB(t, es).DB(), "")
			require.NoError(t, err)
			saver, ok := s.(TxSaver)
			require.True(t, ok)

			coll := makeTestQuiver("github.com/u/r@v1")
			err = es.Atomically(context.Background(), func(tx asynxModels.Tx) error {
				if err := saver.SaveTx(context.Background(), tx, coll); err != nil {
					return err
				}
				if tc.failAfter {
					return errStoreFailure
				}
				return nil
			})

			got, getErr := s.Get(context.Background(), coll.Namespace)
			require.NoError(t, getErr)
			if tc.wantRow {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, coll.Namespace, got.Namespace)
				return
			}
			require.ErrorIs(t, err, errStoreFailure)
			assert.Nil(t, got)
		})
	}
}

func TestHostedStore_SaveTx_Errors(t *testing.T) {
	es := openHost(t)
	s, err := NewHosted(hostDB(t, es).DB(), "")
	require.NoError(t, err)
	saver := s.(TxSaver)

	t.Run("tx not backed by sql", func(t *testing.T) {
		err := saver.SaveTx(context.Background(), nonSQLTx{}, makeTestQuiver("github.com/u/r@v1"))
		require.ErrorIs(t, err, eventstore.ErrNotSQLTx)
	})

	t.Run("store without a host cannot save in a transaction", func(t *testing.T) {
		plain, err := New(":memory:")
		require.NoError(t, err)
		_, ok := plain.(TxSaver)
		assert.False(t, ok)
	})

	t.Run("write failure", func(t *testing.T) {
		err := es.Atomically(context.Background(), func(tx asynxModels.Tx) error {
			sqlTx, ok := tx.Native().(*sql.Tx)
			require.True(t, ok)
			_, err := sqlTx.ExecContext(context.Background(), "DROP TABLE collections")
			require.NoError(t, err)
			return saver.SaveTx(context.Background(), tx, makeTestQuiver("github.com/u/r@v1"))
		})
		require.Error(t, err)
	})
}

func TestHostedStore_CloseLeavesTheHostOpen(t *testing.T) {
	es := openHost(t)
	s, err := NewHosted(hostDB(t, es).DB(), "")
	require.NoError(t, err)

	require.NoError(t, s.Close())
	_, err = es.ListAggregateIDs(context.Background())
	require.NoError(t, err)
}

func TestHostedStore_ImportsTheLegacyFileOnce(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "collections.db")
	legacy, err := New(legacyPath)
	require.NoError(t, err)
	require.NoError(t, legacy.Save(context.Background(), makeTestQuiver("github.com/u/a@v1")))
	require.NoError(t, legacy.Save(context.Background(), makeTestQuiver("github.com/u/b@v1")))
	require.NoError(t, legacy.Close())

	es := openHost(t)
	s, err := NewHosted(hostDB(t, es).DB(), legacyPath)
	require.NoError(t, err)

	all, err := s.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 2)

	_, statErr := os.Stat(legacyPath)
	assert.True(t, os.IsNotExist(statErr), "the legacy file is moved aside")
	_, statErr = os.Stat(legacyPath + ".migrated")
	assert.NoError(t, statErr, "the legacy file is kept as a backup")

	again, err := NewHosted(hostDB(t, es).DB(), legacyPath)
	require.NoError(t, err)
	all, err = again.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestHostedStore_LegacyImport_ExistingRowsWin(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "collections.db")
	legacy, err := New(legacyPath)
	require.NoError(t, err)
	old := makeTestQuiver("github.com/u/a@v1")
	old.FailedArrows = nil
	require.NoError(t, legacy.Save(context.Background(), old))
	require.NoError(t, legacy.Close())

	es := openHost(t)
	host := hostDB(t, es).DB()
	first, err := NewHosted(host, "")
	require.NoError(t, err)
	fresh := makeTestQuiver("github.com/u/a@v1")
	fresh.FailedArrows = []domain.Namespace{"github.com/u/x@v1"}
	require.NoError(t, first.Save(context.Background(), fresh))

	s, err := NewHosted(host, legacyPath)
	require.NoError(t, err)
	got, err := s.Get(context.Background(), fresh.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Len(t, got.FailedArrows, 1)
}

func TestHostedStore_LegacyImport_Errors(t *testing.T) {
	testCases := []struct {
		name string
		path func(t *testing.T) string
	}{
		{name: "missing file is nothing to import", path: func(t *testing.T) string {
			return filepath.Join(t.TempDir(), "absent.db")
		}},
		{name: "file with no table imports nothing", path: func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "empty.db")
			require.NoError(t, os.WriteFile(p, nil, 0o600))
			return p
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			es := openHost(t)
			s, err := NewHosted(hostDB(t, es).DB(), tc.path(t))
			require.NoError(t, err)
			all, err := s.List(context.Background())
			require.NoError(t, err)
			assert.Empty(t, all)
		})
	}

	t.Run("unreadable legacy file fails", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "garbage.db")
		require.NoError(t, os.WriteFile(p, []byte("this is not a sqlite database at all, just text"), 0o600))
		es := openHost(t)
		_, err := NewHosted(hostDB(t, es).DB(), p)
		require.Error(t, err)
	})

	t.Run("a path that cannot be stat'd fails", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("windows reports a path under a file as not found")
		}
		es := openHost(t)
		blocker := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(blocker, nil, 0o600))
		_, err := NewHosted(hostDB(t, es).DB(), filepath.Join(blocker, "collections.db"))
		require.Error(t, err)
	})
}

type nonSQLTx struct{ asynxModels.Tx }

func (nonSQLTx) Native() any { return "not a connection" }
