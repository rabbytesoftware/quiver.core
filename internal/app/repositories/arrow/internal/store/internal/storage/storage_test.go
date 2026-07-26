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

func newTestDB(t *testing.T) *gormdb.DB {
	t.Helper()
	db, err := adapterSQLite.OpenDB(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	return db
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
				installed_ref TEXT, resolved_branch TEXT, installed_at INTEGER,
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
	db, err := adapterSQLite.OpenDB(path)
	require.NoError(t, err)
	// Force the file to exist as a valid but unmigrated sqlite database.
	require.NoError(t, db.Exec(`CREATE TABLE seed (x INTEGER)`).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	ro, err := adapterSQLite.OpenDB(readOnlyDSN(path))
	require.NoError(t, err)

	require.ErrorContains(t, storage.NewSchema(ro), "storage: migrate")
}

func TestNewSchema_CreateFTSError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	db, err := adapterSQLite.OpenDB(path)
	require.NoError(t, err)
	require.NoError(t, storage.NewSchema(db))
	// Leave the GORM tables migrated but the FTS table absent, so reopening
	// read-only gets past AutoMigrate and fails on the virtual table DDL.
	require.NoError(t, db.Exec(`DROP TABLE catalog_arrows_fts`).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	ro, err := adapterSQLite.OpenDB(readOnlyDSN(path))
	require.NoError(t, err)

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
