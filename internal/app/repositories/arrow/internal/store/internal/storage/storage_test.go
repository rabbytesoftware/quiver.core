package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store/internal/storage"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func newTestStoreWithDB(t *testing.T) (*gormdb.DB, storage.Store) {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	s, err := storage.New(db)
	require.NoError(t, err)
	return db, s
}

func newTestStore(t *testing.T) storage.Store {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	s, err := storage.New(db)
	require.NoError(t, err)
	return s
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

	vm.Metadata.Name = "Updated"
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

func TestUnmarshal_InvalidJSON_Error(t *testing.T) {
	_, err := storage.Unmarshal([]byte("not json"))
	require.Error(t, err)
}

func TestFindByKey_CorruptData_Error(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/corrupt@v1.0.0")

	// Save valid data first (to create the row), then overwrite with corrupt JSON.
	err := s.Save(context.Background(), testViewModel(ns))
	require.NoError(t, err)

	// Corrupt the data column directly.
	db.Exec(
		"UPDATE catalog_arrows SET data = ? WHERE namespace = ?",
		[]byte("not valid json"),
		ns.BareNamespace().String(),
	)

	_, err = s.FindByKey(context.Background(), ns.BareNamespace().String())
	require.Error(t, err)
}

func TestFindAll_CorruptData_Error(t *testing.T) {
	db, s := newTestStoreWithDB(t)
	ns := domain.Namespace("github.com/user/corrupt2@v1.0.0")

	err := s.Save(context.Background(), testViewModel(ns))
	require.NoError(t, err)

	db.Exec(
		"UPDATE catalog_arrows SET data = ? WHERE namespace = ?",
		[]byte("not valid json"),
		ns.BareNamespace().String(),
	)

	_, err = s.FindAll(context.Background())
	require.Error(t, err)
}

func TestNew_DBClosed_Error(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

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
