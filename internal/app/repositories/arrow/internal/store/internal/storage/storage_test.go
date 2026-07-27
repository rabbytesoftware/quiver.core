package storage_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store/internal/storage"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

// openTestDB opens dsn and registers its close in the same breath. Every handle
// a test takes has to be released before t.TempDir removes the directory:
// t.Cleanup runs LIFO, so closing here is ordered ahead of the removal. POSIX
// unlinks an open file happily, which is why leaving this out looks harmless —
// Windows refuses, and the test fails on its own cleanup.
func openTestDB(
	t *testing.T,
	dsn string,
) *gormdb.DB {
	t.Helper()
	db, err := adapterSQLite.OpenDB(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapterSQLite.CloseDB(db) })
	return db
}

// newTestDB backs the store with a real file rather than :memory:, because the
// schema under test is on-disk behaviour.
func newTestDB(t *testing.T) *gormdb.DB {
	t.Helper()
	return openTestDB(t, filepath.Join(t.TempDir(), "store.db"))
}

func TestNewSchema_CreatesFTSTable(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, storage.NewSchema(db))

	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM sqlite_master WHERE name = 'catalog_arrows_fts'`,
	).Scan(&n).Error)
	require.Equal(t, int64(1), n)
}

func TestNewSchema_CreatesEveryTable(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, storage.NewSchema(db))

	for _, table := range []string{
		"catalog_arrows",
		"catalog_arrow_versions",
		"catalog_arrow_tags",
		"catalog_arrow_os",
	} {
		var n int64
		require.NoError(t, db.Raw(
			`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&n).Error)
		assert.Equal(t, int64(1), n, "table %s must exist", table)
	}
}

func TestNewSchema_IsIdempotent(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, storage.NewSchema(db))
	require.NoError(t, storage.NewSchema(db))
}

// TestNewSchema_ColumnsAreStable pins every column name and its position.
// AutoMigrate only ever adds columns, so a renamed one becomes a second, empty
// column and orphans the old data without raising an error. Refactors that move
// fields between row structs and embedded domain types have to leave this list
// untouched.
func TestNewSchema_ColumnsAreStable(t *testing.T) {
	testCases := []struct {
		name    string
		table   string
		columns []string
	}{
		{
			name:  "catalog_arrows",
			table: "catalog_arrows",
			columns: []string{
				"namespace", "name", "description", "license", "url",
				"icon", "banner", "provenance", "user_installed", "updated_at",
			},
		},
		{
			name:  "catalog_arrow_versions",
			table: "catalog_arrow_versions",
			columns: []string{
				"namespace", "ref", "installed_ref", "installed_at",
				"user_installed", "manifest",
			},
		},
		{
			name:    "catalog_arrow_tags",
			table:   "catalog_arrow_tags",
			columns: []string{"namespace", "tag"},
		},
		{
			name:    "catalog_arrow_os",
			table:   "catalog_arrow_os",
			columns: []string{"namespace", "ref", "os"},
		},
	}

	db := newTestDB(t)
	require.NoError(t, storage.NewSchema(db))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.columns, tableColumns(t, db, tc.table))
		})
	}
}

// tableColumns returns the column names of table in declaration order.
func tableColumns(
	t *testing.T,
	db *gormdb.DB,
	table string,
) []string {
	t.Helper()
	var columns []string
	require.NoError(t, db.Raw(
		`SELECT name FROM pragma_table_info(?) ORDER BY cid`, table,
	).Scan(&columns).Error)
	return columns
}

func newTestStoreWithDB(t *testing.T) (*gormdb.DB, storage.Store) {
	t.Helper()
	db := newTestDB(t)
	s, err := storage.New(db)
	require.NoError(t, err)
	return db, s
}

func newTestStore(t *testing.T) storage.Store {
	t.Helper()
	_, s := newTestStoreWithDB(t)
	return s
}

func testArrow(ns domain.Namespace) domain.Arrow {
	return domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{
			Name:        "Chromium",
			Description: "A fast web browser",
			License:     "BSD-3-Clause",
			URL:         "https://example.test/chromium",
			Tags:        []string{"browser", "web"},
			Media:       domain.ArrowMedia{Icon: "icon.png", Banner: "banner.png"},
		},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64:  {},
			domain.OSDarwinARM64: {},
		},
		InstalledRef: "^1.0.0",
		InstalledAt:  time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
	}
}

func richViewModel(ns domain.Namespace) storage.ViewModel {
	arrow := testArrow(ns)
	return storage.ViewModel{
		Namespace: ns.BareNamespace(),
		Metadata:  arrow,
		Versions:  []storage.VersionRef{{Namespace: ns, Metadata: arrow}},
	}
}

func countRows(
	t *testing.T,
	db *gormdb.DB,
	table string,
) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM `+table).Scan(&n).Error)
	return n
}

func testViewModel(ns domain.Namespace) storage.ViewModel {
	return storage.ViewModel{
		Namespace: ns.BareNamespace(),
		Metadata:  domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Test"}},
		Versions: []storage.VersionRef{
			{
				Namespace: ns,
				Metadata:  domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Test"}},
			},
		},
	}
}

func TestSave_AndFindByKey(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	vm := testViewModel(ns)

	err := s.Save(context.Background(), vm)
	require.NoError(t, err)

	found, err := s.FindByKey(context.Background(), ns.BareNamespace().String())
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Test", found.Metadata.Name)
}

func TestFindByKey_NotFound(t *testing.T) {
	s := newTestStore(t)

	found, err := s.FindByKey(context.Background(), "github.com/nobody/pkg")
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestSave_Upsert(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	vm := testViewModel(ns)

	err := s.Save(context.Background(), vm)
	require.NoError(t, err)

	// Metadata is derived from the version rows, so a re-save has to carry the
	// new name on the version it came from.
	vm.Metadata.Name = "Updated"
	vm.Versions[0].Metadata.Name = "Updated"
	err = s.Save(context.Background(), vm)
	require.NoError(t, err)

	found, err := s.FindByKey(context.Background(), ns.BareNamespace().String())
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Updated", found.Metadata.Name)
}

func TestFindAll_Empty(t *testing.T) {
	s := newTestStore(t)

	all, err := s.FindAll(context.Background())
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestFindAll_MultipleItems(t *testing.T) {
	s := newTestStore(t)
	ns1 := domain.Namespace("github.com/user/pkg1@v1.0.0")
	ns2 := domain.Namespace("github.com/user/pkg2@v1.0.0")

	err := s.Save(context.Background(), testViewModel(ns1))
	require.NoError(t, err)
	err = s.Save(context.Background(), testViewModel(ns2))
	require.NoError(t, err)

	all, err := s.FindAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")

	err := s.Save(context.Background(), testViewModel(ns))
	require.NoError(t, err)

	err = s.Delete(context.Background(), ns.BareNamespace().String())
	require.NoError(t, err)

	found, err := s.FindByKey(context.Background(), ns.BareNamespace().String())
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestDelete_NonExistent(t *testing.T) {
	s := newTestStore(t)

	// Deleting non-existent key should not error
	err := s.Delete(context.Background(), "github.com/nobody/pkg")
	require.NoError(t, err)
}

// ─── unmarshal error paths ─────────────────────────────────────────────────────

func corruptManifest(
	t *testing.T,
	db *gormdb.DB,
	ns domain.Namespace,
) {
	t.Helper()
	require.NoError(t, db.Exec(
		"UPDATE catalog_arrow_versions SET manifest = ? WHERE namespace = ?",
		[]byte("not valid json"),
		ns.BareNamespace().String(),
	).Error)
}

func TestFindByKey_CorruptData_Error(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/corrupt@v1.0.0")

	// Save valid data first (to create the row), then overwrite with corrupt JSON.
	err := s.Save(context.Background(), testViewModel(ns))
	require.NoError(t, err)

	corruptManifest(t, db, ns)

	_, err = s.FindByKey(context.Background(), ns.BareNamespace().String())
	require.ErrorContains(t, err, "unmarshal")
}

func TestFindAll_CorruptData_Error(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/corrupt2@v1.0.0")

	err := s.Save(context.Background(), testViewModel(ns))
	require.NoError(t, err)

	corruptManifest(t, db, ns)

	_, err = s.FindAll(context.Background())
	require.ErrorContains(t, err, "unmarshal")
}

func TestSave_CorruptStoredManifest_RefreshParentError(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/corrupt3@v1.0.0")

	require.NoError(t, s.Save(context.Background(), richViewModel(ns)))
	corruptManifest(t, db, ns)

	// A narrow write re-reads the stored version rows to rebuild the parent row,
	// so the corrupt sibling manifest surfaces here.
	other := ns.WithRef("v2.0.0")
	err := s.SaveVersion(context.Background(), other, domain.Arrow{Namespace: other})
	require.ErrorContains(t, err, "unmarshal")
}

func TestNew_DBClosed_Error(t *testing.T) {
	db := newTestDB(t)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = storage.New(db)
	require.Error(t, err)
}

func TestFindByKey_DBClosed_Error(t *testing.T) {
	db, s := newTestStoreWithDB(t)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = s.FindByKey(context.Background(), "github.com/user/pkg")
	require.Error(t, err)
}

func TestFindAll_DBClosed_Error(t *testing.T) {
	db, s := newTestStoreWithDB(t)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = s.FindAll(context.Background())
	require.Error(t, err)
}

// ─── relational shape ──────────────────────────────────────────────────────────

func TestStorage_SaveAndFindByKey_RoundTrips(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/user/chromium@v1.0.0")
	vm := richViewModel(ns)

	require.NoError(t, s.Save(context.Background(), vm))

	found, err := s.FindByKey(context.Background(), ns.BareNamespace().String())
	require.NoError(t, err)
	require.NotNil(t, found)

	assert.Equal(t, ns.BareNamespace(), found.Namespace)
	assert.Equal(t, "Chromium", found.Metadata.Name)
	assert.Equal(t, "A fast web browser", found.Metadata.Description)
	assert.Equal(t, "BSD-3-Clause", found.Metadata.License)
	assert.Equal(t, "https://example.test/chromium", found.Metadata.URL)
	assert.Equal(t, []string{"browser", "web"}, found.Metadata.Tags)
	assert.Equal(t, "icon.png", found.Metadata.Media.Icon)
	assert.Equal(t, "banner.png", found.Metadata.Media.Banner)
	assert.Equal(t, "^1.0.0", found.Metadata.InstalledRef)
	assert.True(t, found.Metadata.InstalledAt.Equal(vm.Metadata.InstalledAt))
	assert.Len(t, found.Metadata.Targets, 2)

	require.Len(t, found.Versions, 1)
	assert.Equal(t, ns, found.Versions[0].Namespace)
	assert.Equal(t, "Chromium", found.Versions[0].Metadata.Name)
}

func TestStorage_Save_TwoRefsProduceOneParentAndTwoVersionRows(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	bare := domain.Namespace("github.com/user/chromium")
	v1 := bare.WithRef("v1.0.0")
	v2 := bare.WithRef("v2.0.0")

	require.NoError(t, s.Save(context.Background(), storage.ViewModel{
		Namespace: bare,
		Versions: []storage.VersionRef{
			{Namespace: v1, Metadata: testArrow(v1)},
			{Namespace: v2, Metadata: testArrow(v2)},
		},
	}))

	assert.Equal(t, int64(1), countRows(t, db, "catalog_arrows"))
	assert.Equal(t, int64(2), countRows(t, db, "catalog_arrow_versions"))
	assert.Equal(t, int64(2), countRows(t, db, "catalog_arrow_tags"))
	assert.Equal(t, int64(4), countRows(t, db, "catalog_arrow_os"))
	assert.Equal(t, int64(1), countRows(t, db, "catalog_arrows_fts"))

	found, err := s.FindByKey(context.Background(), bare.String())
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Len(t, found.Versions, 2)
}

func TestStorage_Save_IsIdempotent(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/chromium@v1.0.0")

	for range 3 {
		require.NoError(t, s.Save(context.Background(), richViewModel(ns)))
	}

	assert.Equal(t, int64(1), countRows(t, db, "catalog_arrows"))
	assert.Equal(t, int64(1), countRows(t, db, "catalog_arrow_versions"))
	assert.Equal(t, int64(2), countRows(t, db, "catalog_arrow_tags"))
	assert.Equal(t, int64(2), countRows(t, db, "catalog_arrow_os"))
	assert.Equal(t, int64(1), countRows(t, db, "catalog_arrows_fts"))
}

func TestStorage_FindByKey_UnknownReturnsNilNilNotError(t *testing.T) {
	s := newTestStore(t)

	found, err := s.FindByKey(context.Background(), "github.com/nobody/nothing")
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestStorage_FindAll_ReturnsEverySaved(t *testing.T) {
	s := newTestStore(t)
	first := domain.Namespace("github.com/user/alpha@v1.0.0")
	second := domain.Namespace("github.com/user/beta@v1.0.0")

	require.NoError(t, s.Save(context.Background(), richViewModel(first)))
	require.NoError(t, s.Save(context.Background(), richViewModel(second)))

	all, err := s.FindAll(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 2)

	assert.Equal(t, first.BareNamespace(), all[0].Namespace)
	assert.Equal(t, second.BareNamespace(), all[1].Namespace)
	assert.Len(t, all[0].Versions, 1)
	assert.Equal(t, "Chromium", all[1].Metadata.Name)
}

func TestStorage_Save_ProvenanceInstalledWhenUserInstalled(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/user/chromium@v1.0.0")
	vm := richViewModel(ns)
	vm.Versions[0].Metadata.UserInstalled = true

	require.NoError(t, s.Save(context.Background(), vm))

	found, err := s.FindByKey(context.Background(), ns.BareNamespace().String())
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, storage.ProvenanceInstalled, found.Provenance)
}

func TestStorage_Save_ProvenanceDependencyOtherwise(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/user/chromium@v1.0.0")

	require.NoError(t, s.Save(context.Background(), richViewModel(ns)))

	found, err := s.FindByKey(context.Background(), ns.BareNamespace().String())
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, storage.ProvenanceDependency, found.Provenance)
}

func TestStorage_Save_UpdatesTagsAndOSOnResave(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/chromium@v1.0.0")

	require.NoError(t, s.Save(context.Background(), richViewModel(ns)))

	next := richViewModel(ns)
	next.Versions[0].Metadata.Tags = []string{"editor"}
	next.Versions[0].Metadata.Targets = map[domain.OS]domain.Target{domain.OSWindowsAMD64: {}}
	require.NoError(t, s.Save(context.Background(), next))

	var tags []string
	require.NoError(t, db.Raw(`SELECT tag FROM catalog_arrow_tags`).Scan(&tags).Error)
	assert.Equal(t, []string{"editor"}, tags)

	var oses []string
	require.NoError(t, db.Raw(`SELECT os FROM catalog_arrow_os`).Scan(&oses).Error)
	assert.Equal(t, []string{"windows/amd64"}, oses)
}

func TestStorage_Save_DroppedVersionIsRemoved(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	bare := domain.Namespace("github.com/user/chromium")
	v1 := bare.WithRef("v1.0.0")
	v2 := bare.WithRef("v2.0.0")

	require.NoError(t, s.Save(context.Background(), storage.ViewModel{
		Namespace: bare,
		Versions: []storage.VersionRef{
			{Namespace: v1, Metadata: testArrow(v1)},
			{Namespace: v2, Metadata: testArrow(v2)},
		},
	}))

	require.NoError(t, s.Save(context.Background(), storage.ViewModel{
		Namespace: bare,
		Versions:  []storage.VersionRef{{Namespace: v2, Metadata: testArrow(v2)}},
	}))

	assert.Equal(t, int64(1), countRows(t, db, "catalog_arrow_versions"))
	assert.Equal(t, int64(2), countRows(t, db, "catalog_arrow_os"))
}

func TestStorage_FindByKey_PrefersUserInstalledThenNewest(t *testing.T) {
	s := newTestStore(t)
	bare := domain.Namespace("github.com/user/chromium")

	older := testArrow(bare.WithRef("v1.0.0"))
	older.Name = "old"
	older.InstalledAt = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	newer := testArrow(bare.WithRef("v2.0.0"))
	newer.Name = "new"
	newer.InstalledAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	require.NoError(t, s.Save(context.Background(), storage.ViewModel{
		Namespace: bare,
		Versions: []storage.VersionRef{
			{Namespace: older.Namespace, Metadata: older},
			{Namespace: newer.Namespace, Metadata: newer},
		},
	}))

	found, err := s.FindByKey(context.Background(), bare.String())
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "new", found.Metadata.Name, "newest installed_at wins when neither is user-installed")

	older.UserInstalled = true
	require.NoError(t, s.Save(context.Background(), storage.ViewModel{
		Namespace: bare,
		Versions: []storage.VersionRef{
			{Namespace: older.Namespace, Metadata: older},
			{Namespace: newer.Namespace, Metadata: newer},
		},
	}))

	found, err = s.FindByKey(context.Background(), bare.String())
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "old", found.Metadata.Name, "user-installed outranks a newer install")
}

func TestStorage_Save_NoVersionsStillCreatesParent(t *testing.T) {
	s := newTestStore(t)
	bare := domain.Namespace("github.com/user/empty")

	require.NoError(t, s.Save(context.Background(), storage.ViewModel{Namespace: bare}))

	found, err := s.FindByKey(context.Background(), bare.String())
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Empty(t, found.Versions)
	assert.Equal(t, domain.Arrow{}, found.Metadata)
}

func TestStorage_Save_ReflessNamespaceUsesEmptyRef(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	bare := domain.Namespace("github.com/user/refless")

	require.NoError(t, s.Save(context.Background(), storage.ViewModel{
		Namespace: bare,
		Versions:  []storage.VersionRef{{Namespace: bare, Metadata: testArrow(bare)}},
	}))

	var refs []string
	require.NoError(t, db.Raw(`SELECT ref FROM catalog_arrow_versions`).Scan(&refs).Error)
	assert.Equal(t, []string{""}, refs)

	found, err := s.FindByKey(context.Background(), bare.String())
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Len(t, found.Versions, 1)
	assert.Equal(t, bare, found.Versions[0].Namespace)
}

func TestStorage_Delete_RemovesEveryTable(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/chromium@v1.0.0")

	require.NoError(t, s.Save(context.Background(), richViewModel(ns)))
	require.NoError(t, s.Delete(context.Background(), ns.BareNamespace().String()))

	for _, table := range []string{
		"catalog_arrows",
		"catalog_arrow_versions",
		"catalog_arrow_tags",
		"catalog_arrow_os",
		"catalog_arrows_fts",
	} {
		assert.Equal(t, int64(0), countRows(t, db, table), "table %s must be empty", table)
	}
}

func TestStorage_Save_MarshalError(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/user/broken@v1.0.0")

	arrow := testArrow(ns)
	arrow.Targets = map[domain.OS]domain.Target{
		domain.OSLinuxAMD64: {
			Lifecycle: domain.TargetLifecycle{Install: step.StepList{unmarshalableStep{}}},
		},
	}

	err := s.Save(context.Background(), storage.ViewModel{
		Namespace: ns.BareNamespace(),
		Versions:  []storage.VersionRef{{Namespace: ns, Metadata: arrow}},
	})
	require.ErrorContains(t, err, "storage: marshal manifest")
}

// unmarshalableStep is a domain step whose JSON encoding always fails, so the
// manifest marshal error path is reachable from a test.
type unmarshalableStep struct{}

func (unmarshalableStep) Type() step.StepType        { return step.StepTypeRun }
func (unmarshalableStep) Title() string              { return "boom" }
func (unmarshalableStep) ExitOnFailure() bool        { return true }
func (unmarshalableStep) Resolve(_ string) step.Step { return unmarshalableStep{} }
func (unmarshalableStep) MarshalJSON() ([]byte, error) {
	return nil, errBadStep
}

var errBadStep = errors.New("bad step")

// ─── SQL failure paths ─────────────────────────────────────────────────────────

func TestStorage_Save_SQLFailures(t *testing.T) {
	testCases := []struct {
		name    string
		drop    string
		wantErr string
	}{
		{name: "version table missing", drop: "catalog_arrow_versions", wantErr: "clear versions"},
		{name: "os table missing", drop: "catalog_arrow_os", wantErr: "clear os"},
		{name: "arrow table missing", drop: "catalog_arrows", wantErr: "upsert arrow"},
		{name: "tag table missing", drop: "catalog_arrow_tags", wantErr: "clear tags"},
		{name: "fts table missing", drop: "catalog_arrows_fts", wantErr: "clear fts"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, s := newTestStoreWithDB(t)
			require.NoError(t, db.Exec("DROP TABLE "+tc.drop).Error)

			ns := domain.Namespace("github.com/user/chromium@v1.0.0")
			err := s.Save(context.Background(), richViewModel(ns))
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestStorage_Save_ChildWriteFailures(t *testing.T) {
	testCases := []struct {
		name    string
		ddl     string
		drop    string
		wantErr string
	}{
		{
			name: "tag rejected",
			drop: "catalog_arrow_tags",
			ddl: `CREATE TABLE catalog_arrow_tags (
				namespace TEXT, tag TEXT CHECK (tag <> 'browser'),
				PRIMARY KEY (namespace, tag))`,
			wantErr: "write tag",
		},
		{
			name: "os rejected",
			drop: "catalog_arrow_os",
			ddl: `CREATE TABLE catalog_arrow_os (
				namespace TEXT, ref TEXT, os TEXT CHECK (os <> 'darwin/arm64'),
				PRIMARY KEY (namespace, ref, os))`,
			wantErr: "write os",
		},
		{
			name: "fts rejected",
			drop: "catalog_arrows_fts",
			ddl: `CREATE TABLE catalog_arrows_fts (
				namespace TEXT, name TEXT CHECK (name <> 'Chromium'),
				description TEXT, tags TEXT)`,
			wantErr: "write fts",
		},
		{
			name: "version rejected",
			drop: "catalog_arrow_versions",
			ddl: `CREATE TABLE catalog_arrow_versions (
				namespace TEXT, ref TEXT CHECK (ref <> 'v1.0.0'),
				installed_ref TEXT, installed_at INTEGER,
				user_installed NUMERIC, manifest BLOB,
				PRIMARY KEY (namespace, ref))`,
			wantErr: "upsert version",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, s := newTestStoreWithDB(t)
			require.NoError(t, db.Exec("DROP TABLE "+tc.drop).Error)
			require.NoError(t, db.Exec(tc.ddl).Error)

			ns := domain.Namespace("github.com/user/chromium@v1.0.0")
			err := s.Save(context.Background(), richViewModel(ns))
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestStorage_Delete_SQLFailures(t *testing.T) {
	testCases := []struct {
		name    string
		drop    string
		wantErr string
	}{
		{name: "arrow table missing", drop: "catalog_arrows", wantErr: "delete arrow"},
		{name: "version table missing", drop: "catalog_arrow_versions", wantErr: "delete versions"},
		{name: "tag table missing", drop: "catalog_arrow_tags", wantErr: "delete tags"},
		{name: "os table missing", drop: "catalog_arrow_os", wantErr: "delete os"},
		{name: "fts table missing", drop: "catalog_arrows_fts", wantErr: "delete fts"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, s := newTestStoreWithDB(t)
			require.NoError(t, db.Exec("DROP TABLE "+tc.drop).Error)

			err := s.Delete(context.Background(), "github.com/user/chromium")
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestStorage_FindByKey_LoadVersionsError(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/chromium@v1.0.0")

	require.NoError(t, s.Save(context.Background(), richViewModel(ns)))
	require.NoError(t, db.Exec(`DROP TABLE catalog_arrow_versions`).Error)

	_, err := s.FindByKey(context.Background(), ns.BareNamespace().String())
	require.ErrorContains(t, err, "load versions")
}

func TestStorage_SaveVersion_UpsertsOneRow(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	bare := domain.Namespace("github.com/user/chromium")
	v1 := bare.WithRef("v1.0.0")
	v2 := bare.WithRef("v2.0.0")

	require.NoError(t, s.SaveVersion(context.Background(), v1, testArrow(v1)))
	require.NoError(t, s.SaveVersion(context.Background(), v2, testArrow(v2)))

	assert.Equal(t, int64(1), countRows(t, db, "catalog_arrows"))
	assert.Equal(t, int64(2), countRows(t, db, "catalog_arrow_versions"))

	found, err := s.FindByKey(context.Background(), bare.String())
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Len(t, found.Versions, 2)
}

func TestStorage_SaveVersion_ReplacesSameRef(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/chromium@v1.0.0")

	require.NoError(t, s.SaveVersion(context.Background(), ns, testArrow(ns)))

	renamed := testArrow(ns)
	renamed.Name = "Chromium Beta"
	require.NoError(t, s.SaveVersion(context.Background(), ns, renamed))

	assert.Equal(t, int64(1), countRows(t, db, "catalog_arrow_versions"))

	found, err := s.FindByKey(context.Background(), ns.BareNamespace().String())
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Chromium Beta", found.Metadata.Name)
}

func TestStorage_SaveVersion_SQLFailure(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	require.NoError(t, db.Exec(`DROP TABLE catalog_arrow_versions`).Error)

	ns := domain.Namespace("github.com/user/chromium@v1.0.0")
	err := s.SaveVersion(context.Background(), ns, testArrow(ns))
	require.ErrorContains(t, err, "upsert version")
}

func TestStorage_SaveVersion_MarshalError(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/user/broken@v1.0.0")

	arrow := testArrow(ns)
	arrow.Targets = map[domain.OS]domain.Target{
		domain.OSLinuxAMD64: {
			Lifecycle: domain.TargetLifecycle{Install: step.StepList{unmarshalableStep{}}},
		},
	}

	err := s.SaveVersion(context.Background(), ns, arrow)
	require.ErrorContains(t, err, "storage: marshal manifest")
}

// readOnlyDSN builds a DSN that opens path read-only, so writes fail without
// depending on filesystem permissions (which root would bypass).
func readOnlyDSN(path string) string { return "file:" + path + "?mode=ro" }

func TestNewSchema_MigrateError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	db := openTestDB(t, path)
	// Force the file to exist as a valid but unmigrated sqlite database.
	require.NoError(t, db.Exec(`CREATE TABLE seed (x INTEGER)`).Error)
	require.NoError(t, adapterSQLite.CloseDB(db))

	ro := openTestDB(t, readOnlyDSN(path))

	require.ErrorContains(t, storage.NewSchema(ro), "storage: migrate")
}

func TestNewSchema_CreateFTSError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	db := openTestDB(t, path)
	require.NoError(t, storage.NewSchema(db))
	// Leave the GORM tables migrated but the FTS table absent, so reopening
	// read-only gets past AutoMigrate and fails on the virtual table DDL.
	require.NoError(t, db.Exec(`DROP TABLE catalog_arrows_fts`).Error)
	require.NoError(t, adapterSQLite.CloseDB(db))

	ro := openTestDB(t, readOnlyDSN(path))

	require.ErrorContains(t, storage.NewSchema(ro), "storage: create fts")
}

func TestStorage_SaveVersion_ClearOSError(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	require.NoError(t, db.Exec(`DROP TABLE catalog_arrow_os`).Error)

	ns := domain.Namespace("github.com/user/chromium@v1.0.0")
	err := s.SaveVersion(context.Background(), ns, testArrow(ns))
	require.ErrorContains(t, err, "clear os")
}

func TestStorage_Save_RefreshParentLoadError(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	require.NoError(t, db.Exec(`DROP TABLE catalog_arrow_versions`).Error)
	// A version table without the ranking columns: clearing succeeds, but the
	// parent refresh cannot order the rows.
	require.NoError(t, db.Exec(`CREATE TABLE catalog_arrow_versions (namespace TEXT)`).Error)

	err := s.Save(context.Background(), storage.ViewModel{
		Namespace: domain.Namespace("github.com/user/chromium"),
	})
	require.ErrorContains(t, err, "load versions")
}

func TestStorage_Save_DuplicateTagsWrittenOnce(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/chromium@v1.0.0")

	vm := richViewModel(ns)
	vm.Versions[0].Metadata.Tags = []string{"browser", "browser", "web"}
	require.NoError(t, s.Save(context.Background(), vm))

	var tags []string
	require.NoError(t, db.Raw(`SELECT tag FROM catalog_arrow_tags ORDER BY tag`).Scan(&tags).Error)
	assert.Equal(t, []string{"browser", "web"}, tags)
}

// ─── search ────────────────────────────────────────────────────────────────────

func seedSearchable(
	t *testing.T,
	s storage.Store,
	ns domain.Namespace,
	mutate func(*domain.Arrow),
) {
	t.Helper()
	arrow := testArrow(ns)
	if mutate != nil {
		mutate(&arrow)
	}
	require.NoError(t, s.SaveVersion(context.Background(), ns, arrow))
}

func TestStorage_Search_TrigramMatchesSubstring(t *testing.T) {
	s := newTestStore(t)
	seedSearchable(t, s, "github.com/user/chromium@v1.0.0", nil)

	for _, q := range []string{"chrom", "Chromium", "romi"} {
		rows, err := s.Search(context.Background(), storage.Query{Text: q, Limit: 10})
		require.NoError(t, err, q)
		require.Len(t, rows, 1, "query %q should match the name by trigram", q)
		assert.Equal(t, "Chromium", rows[0].Metadata.Name)
	}
}

func TestStorage_Search_MatchesTagAndDescription(t *testing.T) {
	s := newTestStore(t)
	seedSearchable(t, s, "github.com/user/chromium@v1.0.0", nil)

	for _, q := range []string{"browser", "fast web"} {
		rows, err := s.Search(context.Background(), storage.Query{Text: q, Limit: 10})
		require.NoError(t, err, q)
		require.Len(t, rows, 1, "query %q should match a tag or the description", q)
	}
}

func TestStorage_Search_HyphenatedQueryIsLiteral(t *testing.T) {
	s := newTestStore(t)
	seedSearchable(t, s, "github.com/user/core@v1.0.0", func(a *domain.Arrow) {
		a.Name = "quiver-core"
	})

	rows, err := s.Search(context.Background(), storage.Query{Text: "quiver-core", Limit: 10})
	require.NoError(t, err, "a hyphen is FTS5 query syntax and must not be parsed")
	require.Len(t, rows, 1)
	assert.Equal(t, "quiver-core", rows[0].Metadata.Name)
}

func TestStorage_Search_QuerySyntaxCharactersAreSafe(t *testing.T) {
	s := newTestStore(t)
	seedSearchable(t, s, "github.com/user/chromium@v1.0.0", nil)

	for _, q := range []string{`"`, `*`, `chrom OR`, `NEAR(`, `^`, `chrom AND web`, `a"b"c`} {
		_, err := s.Search(context.Background(), storage.Query{Text: q, Limit: 10})
		require.NoError(t, err, "query %q must not be parsed as FTS5 syntax", q)
	}
}

func TestStorage_Search_OSFilterExcludes(t *testing.T) {
	s := newTestStore(t)
	seedSearchable(t, s, "github.com/user/chromium@v1.0.0", nil)

	match, err := s.Search(context.Background(), storage.Query{
		Text: "chrom", OS: domain.OSLinuxAMD64, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, match, 1)

	none, err := s.Search(context.Background(), storage.Query{
		Text: "chrom", OS: domain.OSWindowsAMD64, Limit: 10,
	})
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestStorage_Search_RespectsLimit(t *testing.T) {
	s := newTestStore(t)
	for _, name := range []string{"one", "two", "three"} {
		seedSearchable(t, s, domain.Namespace("github.com/user/"+name+"@v1.0.0"), nil)
	}

	rows, err := s.Search(context.Background(), storage.Query{Text: "chrom", Limit: 2})
	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

func TestStorage_Search_ZeroLimitUsesDefault(t *testing.T) {
	s := newTestStore(t)
	seedSearchable(t, s, "github.com/user/chromium@v1.0.0", nil)

	rows, err := s.Search(context.Background(), storage.Query{Text: "chrom"})
	require.NoError(t, err)
	assert.Len(t, rows, 1)
}

func TestStorage_Search_EmptyTextReturnsEmptyNotError(t *testing.T) {
	s := newTestStore(t)
	seedSearchable(t, s, "github.com/user/chromium@v1.0.0", nil)

	rows, err := s.Search(context.Background(), storage.Query{Text: "   ", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestStorage_Search_NoMatchReturnsEmptyNotError(t *testing.T) {
	s := newTestStore(t)
	seedSearchable(t, s, "github.com/user/chromium@v1.0.0", nil)

	rows, err := s.Search(context.Background(), storage.Query{Text: "zzzzz", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestStorage_Search_HydratesVersionsAndProvenance(t *testing.T) {
	s := newTestStore(t)
	bare := domain.Namespace("github.com/user/chromium")
	seedSearchable(t, s, bare.WithRef("v1.0.0"), func(a *domain.Arrow) { a.UserInstalled = true })
	seedSearchable(t, s, bare.WithRef("v2.0.0"), nil)

	rows, err := s.Search(context.Background(), storage.Query{Text: "chrom", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	assert.Equal(t, bare, rows[0].Namespace)
	assert.Equal(t, storage.ProvenanceInstalled, rows[0].Provenance)
	assert.Len(t, rows[0].Versions, 2)
	assert.True(t, rows[0].Metadata.UserInstalled)
}

func TestStorage_Search_RanksNameAboveDescription(t *testing.T) {
	s := newTestStore(t)
	seedSearchable(t, s, "github.com/user/mentioned@v1.0.0", func(a *domain.Arrow) {
		a.Name = "Firefox"
		a.Description = "an alternative to chromium"
		a.Tags = nil
	})
	seedSearchable(t, s, "github.com/user/named@v1.0.0", func(a *domain.Arrow) {
		a.Name = "Chromium"
		a.Description = "a browser"
		a.Tags = nil
	})

	rows, err := s.Search(context.Background(), storage.Query{Text: "chromium", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "Chromium", rows[0].Metadata.Name, "a name hit must outrank a description hit")
}

func TestStorage_Search_DeletedArrowIsGone(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/user/chromium@v1.0.0")
	seedSearchable(t, s, ns, nil)

	require.NoError(t, s.Delete(context.Background(), ns.BareNamespace().String()))

	rows, err := s.Search(context.Background(), storage.Query{Text: "chrom", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestStorage_Search_ReflectsRename(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/user/chromium@v1.0.0")
	seedSearchable(t, s, ns, nil)
	seedSearchable(t, s, ns, func(a *domain.Arrow) { a.Name = "Thunderbird" })

	stale, err := s.Search(context.Background(), storage.Query{Text: "chrom", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, stale, "the FTS row must not keep the old name")

	fresh, err := s.Search(context.Background(), storage.Query{Text: "thunder", Limit: 10})
	require.NoError(t, err)
	assert.Len(t, fresh, 1)
}

func TestStorage_Search_QueryError(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	require.NoError(t, db.Exec(`DROP TABLE catalog_arrows_fts`).Error)

	_, err := s.Search(context.Background(), storage.Query{Text: "chrom", Limit: 10})
	require.ErrorContains(t, err, "storage: search")
}

func TestStorage_Search_LoadVersionsError(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	seedSearchable(t, s, "github.com/user/chromium@v1.0.0", nil)
	require.NoError(t, db.Exec(`DROP TABLE catalog_arrow_versions`).Error)

	_, err := s.Search(context.Background(), storage.Query{Text: "chrom", Limit: 10})
	require.ErrorContains(t, err, "load versions")
}

// ─── installed ref ───────────────────────────────────────────────────────────

func installedRefOf(
	t *testing.T,
	db *gormdb.DB,
	bare string,
	ref string,
) string {
	t.Helper()
	var installed string
	require.NoError(t, db.Raw(
		`SELECT installed_ref FROM catalog_arrow_versions WHERE namespace = ? AND ref = ?`,
		bare, ref,
	).Scan(&installed).Error)
	return installed
}

// The catalog schema must not carry a column for a field the aggregate dropped:
// a stale column is a place for a second, disagreeing answer to accumulate.
func TestVersionSchema_HasNoResolvedBranchColumn(t *testing.T) {
	db, _ := newTestStoreWithDB(t)

	var columns []string
	require.NoError(t, db.Raw(
		`SELECT name FROM pragma_table_info('catalog_arrow_versions')`,
	).Scan(&columns).Error)

	assert.NotContains(t, columns, "resolved_branch")
	assert.Contains(t, columns, "installed_ref")
}

func TestSaveVersion_WritesInstalledRef(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")

	arrow := testArrow(ns)
	arrow.InstalledRef = "v1.0.0"
	require.NoError(t, s.SaveVersion(context.Background(), ns, arrow))

	assert.Equal(t, "v1.0.0", installedRefOf(t, db, ns.BareNamespace().String(), "v1.0.0"))
}

// The version upsert uses OnConflict{UpdateAll: true}, so a column the writer
// never populates is reset to "" on every re-save. Nothing fails loudly when
// that happens, so the ref is pinned here explicitly.
func TestInstalledRef_SurvivesVersionResave(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	bare := ns.BareNamespace().String()

	arrow := testArrow(ns)
	arrow.InstalledRef = "v1.0.0"
	require.NoError(t, s.SaveVersion(context.Background(), ns, arrow))
	require.Equal(t, "v1.0.0", installedRefOf(t, db, bare, "v1.0.0"))

	arrow.Name = "Chromium Nightly"
	require.NoError(t, s.SaveVersion(context.Background(), ns, arrow))

	assert.Equal(t, "v1.0.0", installedRefOf(t, db, bare, "v1.0.0"))
}

func TestInstalledRef_SurvivesFullAggregateSave(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")

	arrow := testArrow(ns)
	arrow.InstalledRef = "v1.0.0"
	require.NoError(t, s.Save(context.Background(), storage.ViewModel{
		Namespace: ns.BareNamespace(),
		Metadata:  arrow,
		Versions:  []storage.VersionRef{{Namespace: ns, Metadata: arrow}},
	}))

	assert.Equal(t, "v1.0.0", installedRefOf(t, db, ns.BareNamespace().String(), "v1.0.0"))
}

// The manifest blob is the only path a rebuilt aggregate travels, so the field
// has to survive the JSON round trip as well as the column.
func TestInstalledRef_RoundTripsThroughTheManifestBlob(t *testing.T) {
	s := newTestStore(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")

	arrow := testArrow(ns)
	arrow.InstalledRef = "v1.0.0"
	require.NoError(t, s.SaveVersion(context.Background(), ns, arrow))

	found, err := s.FindByKey(context.Background(), ns.BareNamespace().String())
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Len(t, found.Versions, 1)
	assert.Equal(t, "v1.0.0", found.Versions[0].Metadata.InstalledRef)
}
