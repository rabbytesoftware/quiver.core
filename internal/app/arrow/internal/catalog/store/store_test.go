package store

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errStoreFailure = errors.New("store failure")

type errArrowStore struct{}

func (e *errArrowStore) Save(_ context.Context, _ arrowRow) error { return errStoreFailure }
func (e *errArrowStore) Delete(_ context.Context, _ string) error { return errStoreFailure }
func (e *errArrowStore) FindByKey(_ context.Context, _ string) (*arrowRow, error) {
	return nil, errStoreFailure
}
func (e *errArrowStore) FindAll(_ context.Context) ([]arrowRow, error) {
	return nil, errStoreFailure
}

func makeTestViewModel(
	bareNs string,
	versions []domain.Namespace,
) ArrowViewModel {
	versionRefs := make([]ArrowVersionRef, len(versions))
	for i, vns := range versions {
		versionRefs[i] = ArrowVersionRef{
			Namespace: vns,
			Metadata: domain.Arrow{
				Namespace: vns,
				ArrowMeta: domain.ArrowMeta{
					Name:    "TestArrow",
					Version: "1.0.0",
				},
			},
		}
	}

	return ArrowViewModel{
		Namespace: domain.Namespace(bareNs),
		Metadata:  versionRefs[0].Metadata,
		Versions:  versionRefs,
	}
}

func TestArrowCatalog_SaveAndGet_ReturnsSavedViewModel(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	vm := makeTestViewModel("github.com/org/repo", []domain.Namespace{
		"github.com/org/repo@v1.0",
	})

	err = c.Save(context.Background(), vm)
	require.NoError(t, err)

	got, err := c.Get(context.Background(), vm.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, vm.Namespace, got.Namespace)
	assert.Len(t, got.Versions, 1)
	assert.Equal(t, vm.Versions[0].Namespace, got.Versions[0].Namespace)
}

func TestArrowCatalog_SaveDeleteGet_ReturnsNil(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	vm := makeTestViewModel("github.com/org/repo", []domain.Namespace{
		"github.com/org/repo@v1.0",
	})

	err = c.Save(context.Background(), vm)
	require.NoError(t, err)

	err = c.Delete(context.Background(), vm.Namespace)
	require.NoError(t, err)

	got, err := c.Get(context.Background(), vm.Namespace)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestArrowCatalog_List_ReturnsAllSaved(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	vm1 := makeTestViewModel("github.com/org/repo1", []domain.Namespace{
		"github.com/org/repo1@v1.0",
	})
	vm2 := makeTestViewModel("github.com/org/repo2", []domain.Namespace{
		"github.com/org/repo2@v1.0",
	})

	require.NoError(t, c.Save(context.Background(), vm1))
	require.NoError(t, c.Save(context.Background(), vm2))

	vms, err := c.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, vms, 2)

	namespaces := make([]domain.Namespace, 0, len(vms))
	for _, vm := range vms {
		namespaces = append(namespaces, vm.Namespace)
	}
	assert.Contains(t, namespaces, vm1.Namespace)
	assert.Contains(t, namespaces, vm2.Namespace)
}

func TestArrowCatalog_ListAfterRemove_ReturnsEmpty(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	vm := makeTestViewModel("github.com/org/repo", []domain.Namespace{
		"github.com/org/repo@v1.0",
	})

	require.NoError(t, c.Save(context.Background(), vm))
	require.NoError(t, c.Delete(context.Background(), vm.Namespace))

	vms, err := c.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, vms)
}

func TestNewArrowCatalog_InvalidPath_ReturnsError(t *testing.T) {
	_, err := NewArrowCatalog("/invalid/path/arrows.db")
	assert.Error(t, err)
}

func TestArrowCatalog_Save_InnerError_ReturnsError(t *testing.T) {
	c := &arrowCatalog{inner: &errArrowStore{}}
	vm := makeTestViewModel("github.com/org/repo", []domain.Namespace{
		"github.com/org/repo@v1.0",
	})
	err := c.Save(context.Background(), vm)
	assert.Error(t, err)
}

func TestArrowCatalog_Get_InnerError_ReturnsError(t *testing.T) {
	c := &arrowCatalog{inner: &errArrowStore{}}
	_, err := c.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestArrowCatalog_List_InnerError_ReturnsError(t *testing.T) {
	c := &arrowCatalog{inner: &errArrowStore{}}
	_, err := c.List(context.Background())
	assert.Error(t, err)
}

func TestArrowCatalog_Get_CorruptedViewModel_ReturnsError(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	catalog := c.(*arrowCatalog)
	require.NoError(t, catalog.inner.Save(context.Background(), arrowRow{
		Namespace: "github.com/org/repo",
		ViewModel: "not-valid-json{{{",
	}))

	_, err = c.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestArrowCatalog_List_CorruptedViewModel_ReturnsError(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	catalog := c.(*arrowCatalog)
	require.NoError(t, catalog.inner.Save(context.Background(), arrowRow{
		Namespace: "github.com/org/repo",
		ViewModel: "{invalid",
	}))

	_, err = c.List(context.Background())
	assert.Error(t, err)
}

func TestNewArrowCatalogFromDB_SaveAndGet(t *testing.T) {
	db, err := sqlite.OpenDB(":memory:")
	require.NoError(t, err)

	c, err := NewArrowCatalogFromDB(db)
	require.NoError(t, err)

	vm := makeTestViewModel("github.com/org/repo", []domain.Namespace{
		"github.com/org/repo@v1.0",
	})
	require.NoError(t, c.Save(context.Background(), vm))

	got, err := c.Get(context.Background(), vm.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, vm.Namespace, got.Namespace)
}

func TestNewArrowCatalogFromDB_ClosedDB_ReturnsError(t *testing.T) {
	db, err := sqlite.OpenDB(":memory:")
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = NewArrowCatalogFromDB(db)
	assert.Error(t, err)
}

func TestArrowCatalog_SaveMultipleVersions_AggregatesVersions(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	vm := makeTestViewModel("github.com/org/repo", []domain.Namespace{
		"github.com/org/repo@v1.0",
		"github.com/org/repo@v2.0",
	})

	require.NoError(t, c.Save(context.Background(), vm))

	got, err := c.Get(context.Background(), vm.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Len(t, got.Versions, 2)
	assert.Equal(t, domain.Namespace("github.com/org/repo@v1.0"), got.Versions[0].Namespace)
	assert.Equal(t, domain.Namespace("github.com/org/repo@v2.0"), got.Versions[1].Namespace)
}
