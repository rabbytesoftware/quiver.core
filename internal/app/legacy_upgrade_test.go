package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/adapter"
	adapterSqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/engine"
)

// seedLegacyReadModels writes the read-model files a release before the
// hosted read models kept next to the events: one row each.
func seedLegacyReadModels(t *testing.T, home string) string {
	t.Helper()
	storeDir, err := paths.StoreAt(home)
	require.NoError(t, err)

	statements := map[string][]string{
		"collections.db": {
			"CREATE TABLE collections (namespace TEXT PRIMARY KEY, data TEXT NOT NULL)",
			`INSERT INTO collections VALUES ('github.com/org/legacy', '{"Namespace":"github.com/org/legacy"}')`,
		},
		"device_store.db": {
			"CREATE TABLE auth_devices (id TEXT PRIMARY KEY, label TEXT, token_hash TEXT, state TEXT, paired_at INTEGER, last_seen_at INTEGER)",
			"CREATE UNIQUE INDEX idx_auth_devices_token_hash ON auth_devices(token_hash)",
			"INSERT INTO auth_devices VALUES ('legacy-dev', 'phone', 'legacy-hash', 'active', 1, 2)",
		},
	}
	for file, stmts := range statements {
		db, err := adapterSqlite.OpenDB(filepath.Join(storeDir, file))
		require.NoError(t, err)
		for _, stmt := range stmts {
			require.NoError(t, db.Exec(stmt).Error)
		}
		require.NoError(t, adapterSqlite.CloseDB(db))
	}
	return storeDir
}

func TestNew_UpgradeKeepsTheReadModelsAnEarlierReleaseWrote(t *testing.T) {
	home := t.TempDir()
	storeDir := seedLegacyReadModels(t, home)
	ctx := context.Background()

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = engines.Shutdown(context.Background()) })
	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := New(engines, adapters, WithHomeDir(home))
	require.NoError(t, err)
	release(t, c)

	followed := true
	collections, err := c.Collection.List(ctx, &followed)
	require.NoError(t, err)
	require.Len(t, collections, 1)
	assert.Equal(t, "github.com/org/legacy", collections[0].Namespace.String())

	devices, err := c.Auth.ListDevices(ctx)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "legacy-dev", devices[0].ID)

	for _, file := range []string{"collections.db", "device_store.db"} {
		_, err := os.Stat(filepath.Join(storeDir, file+".migrated"))
		assert.NoError(t, err, "%s is kept as a backup", file)
		_, err = os.Stat(filepath.Join(storeDir, file))
		assert.True(t, os.IsNotExist(err), "%s is not opened again", file)
	}
}
