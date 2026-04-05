package sqlite

import (
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeTestMigrations returns an in-memory FS with a single goose migration
// that creates the test_items table.
func makeTestMigrations() fs.FS {
	return fstest.MapFS{
		"migrations/00001_create_test_items.sql": &fstest.MapFile{
			Data: []byte(`-- +goose Up
CREATE TABLE IF NOT EXISTS test_items (
    id    INTEGER PRIMARY KEY,
    name  TEXT    NOT NULL,
    count INTEGER NOT NULL,
    flag  INTEGER NOT NULL
);
-- +goose Down
DROP TABLE test_items;
`),
		},
	}
}

func TestOpen_InMemory_RunsMigrations(t *testing.T) {
	db, err := Open(":memory:", makeTestMigrations(), "migrations")
	require.NoError(t, err)
	require.NotNil(t, db)

	// Verify the table was created by the migration
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM test_items`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestOpen_PersistentFile_RunsMigrations(t *testing.T) {
	tmp, err := os.CreateTemp("", "open_test_*.db")
	require.NoError(t, err)
	tmp.Close()
	defer os.Remove(tmp.Name())

	db, err := Open(tmp.Name(), makeTestMigrations(), "migrations")
	require.NoError(t, err)
	require.NotNil(t, db)

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM test_items`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestOpen_AppliesMigrationsIdempotently(t *testing.T) {
	tmp, err := os.CreateTemp("", "open_idempotent_*.db")
	require.NoError(t, err)
	tmp.Close()
	defer os.Remove(tmp.Name())

	migrations := makeTestMigrations()

	// Open twice — second run should not fail (migrations already applied)
	_, err = Open(tmp.Name(), migrations, "migrations")
	require.NoError(t, err)

	_, err = Open(tmp.Name(), migrations, "migrations")
	require.NoError(t, err)
}

func TestOpen_InvalidMigrationsDir_ReturnsError(t *testing.T) {
	_, err := Open(":memory:", makeTestMigrations(), "nonexistent_dir")
	assert.Error(t, err)
}
