package sqlite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seededLegacyFile(t *testing.T, rows ...testItem) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := OpenDB(path)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&testItem{}))
	if len(rows) > 0 {
		require.NoError(t, db.Create(&rows).Error)
	}
	require.NoError(t, CloseDB(db))
	return path
}

func hostWithItems(t *testing.T, rows ...testItem) *gorm.DB {
	t.Helper()
	db, err := OpenDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&testItem{}))
	if len(rows) > 0 {
		require.NoError(t, db.Create(&rows).Error)
	}
	t.Cleanup(func() { _ = CloseDB(db) })
	return db
}

func TestImportLegacyFile_CopiesRowsAndSetsTheFileAside(t *testing.T) {
	testCases := []struct {
		name     string
		legacy   []testItem
		existing []testItem
		wantRows []testItem
		wantRead int
	}{
		{
			name:     "legacy rows are copied",
			legacy:   []testItem{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}},
			wantRows: []testItem{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}},
			wantRead: 2,
		},
		{
			name:     "an existing key wins over the legacy row",
			legacy:   []testItem{{ID: 1, Name: "old"}},
			existing: []testItem{{ID: 1, Name: "new"}},
			wantRows: []testItem{{ID: 1, Name: "new"}},
			wantRead: 1,
		},
		{
			name:     "an empty legacy table copies nothing",
			wantRows: []testItem{},
			wantRead: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := seededLegacyFile(t, tc.legacy...)
			host := hostWithItems(t, tc.existing...)

			read, err := ImportLegacyFile[testItem](host, path)
			require.NoError(t, err)
			assert.Equal(t, tc.wantRead, read)

			var got []testItem
			require.NoError(t, host.Order("id").Find(&got).Error)
			assert.Equal(t, tc.wantRows, got)
			assert.NoFileExists(t, path)
			assert.FileExists(t, path+".migrated")
		})
	}
}

func TestImportLegacyFile_NothingToImport(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o600))

	testCases := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "empty path", path: ""},
		{name: "missing file", path: filepath.Join(t.TempDir(), "absent.db")},
		{name: "unreadable path", path: filepath.Join(notADir, "legacy.db"), wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host := hostWithItems(t)

			read, err := ImportLegacyFile[testItem](host, tc.path)
			assert.Zero(t, read)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestImportLegacyFile_FileWithoutTheTable_CopiesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := OpenDB(path)
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE other (id INTEGER)").Error)
	require.NoError(t, CloseDB(db))

	read, err := ImportLegacyFile[testItem](hostWithItems(t), path)
	require.NoError(t, err)
	assert.Zero(t, read)
	assert.FileExists(t, path+".migrated")
}

func TestImportLegacyFile_NotADatabase_Fails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	require.NoError(t, os.WriteFile(path, []byte("this is not sqlite"), 0o600))

	_, err := ImportLegacyFile[testItem](hostWithItems(t), path)
	require.Error(t, err)
	assert.FileExists(t, path, "a file that could not be read stays where it is")
}

func TestImportLegacyFile_RenameFails_KeepsTheFileAndSucceeds(t *testing.T) {
	path := seededLegacyFile(t, testItem{ID: 1, Name: "a"})
	require.NoError(t, os.Mkdir(path+".migrated", 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(path+".migrated", "keep"), []byte("x"), 0o600))
	host := hostWithItems(t)

	read, err := ImportLegacyFile[testItem](host, path)
	require.NoError(t, err)
	assert.Equal(t, 1, read)
	assert.FileExists(t, path)
}

func TestImportLegacyDB_Errors(t *testing.T) {
	legacy := hostWithItems(t, testItem{ID: 1, Name: "a"})

	testCases := []struct {
		name   string
		host   func(t *testing.T) *gorm.DB
		legacy *gorm.DB
	}{
		{
			name: "host without the table",
			host: func(t *testing.T) *gorm.DB {
				db, err := OpenDB(":memory:")
				require.NoError(t, err)
				t.Cleanup(func() { _ = CloseDB(db) })
				return db
			},
			legacy: legacy,
		},
		{
			name: "legacy row that cannot be scanned",
			host: func(t *testing.T) *gorm.DB { return hostWithItems(t) },
			legacy: func() *gorm.DB {
				db, err := OpenDB(":memory:")
				require.NoError(t, err)
				require.NoError(t, db.Exec("CREATE TABLE test_items (id TEXT, name TEXT)").Error)
				require.NoError(t, db.Exec("INSERT INTO test_items VALUES ('not-a-number', 'x')").Error)
				t.Cleanup(func() { _ = CloseDB(db) })
				return db
			}(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ImportLegacyDB[testItem](tc.host(t), tc.legacy)
			require.Error(t, err)
		})
	}
}

func TestImportLegacyDB_NilLegacy_CopiesNothing(t *testing.T) {
	read, err := ImportLegacyDB[testItem](hostWithItems(t), nil)
	require.NoError(t, err)
	assert.Zero(t, read)
}
