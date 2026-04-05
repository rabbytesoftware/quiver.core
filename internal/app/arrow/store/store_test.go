package store

import (
	"context"
	"errors"
	"testing"

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

func makeTestArrow(ns string, name string) domain.Arrow {
	return domain.Arrow{
		Namespace: domain.Namespace(ns),
		Manifest: domain.ArrowManifest{
			Name:    name,
			Version: "1.0.0",
		},
		Removed: false,
	}
}

func TestArrowCatalog_SaveAndGet_ReturnsSavedArrow(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	arrow := makeTestArrow("github.com/org/repo", "MyArrow")

	err = c.Save(context.Background(), arrow)
	require.NoError(t, err)

	got, err := c.Get(context.Background(), arrow.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, arrow.Namespace, got.Namespace)
	assert.Equal(t, arrow.Manifest.Name, got.Manifest.Name)
	assert.Equal(t, arrow.Manifest.Version, got.Manifest.Version)
	assert.Equal(t, arrow.Removed, got.Removed)
}

func TestArrowCatalog_SaveDeleteGet_ReturnsNil(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	arrow := makeTestArrow("github.com/org/repo", "MyArrow")

	err = c.Save(context.Background(), arrow)
	require.NoError(t, err)

	err = c.Delete(context.Background(), arrow.Namespace)
	require.NoError(t, err)

	got, err := c.Get(context.Background(), arrow.Namespace)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestArrowCatalog_List_ReturnsAllSaved(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	a1 := makeTestArrow("github.com/org/repo1", "Arrow1")
	a2 := makeTestArrow("github.com/org/repo2", "Arrow2")

	require.NoError(t, c.Save(context.Background(), a1))
	require.NoError(t, c.Save(context.Background(), a2))

	arrows, err := c.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, arrows, 2)

	namespaces := make([]domain.Namespace, 0, len(arrows))
	for _, a := range arrows {
		namespaces = append(namespaces, a.Namespace)
	}
	assert.Contains(t, namespaces, a1.Namespace)
	assert.Contains(t, namespaces, a2.Namespace)
}

func TestArrowCatalog_ListAfterRemove_ReturnsEmpty(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	arrow := makeTestArrow("github.com/org/repo", "MyArrow")

	require.NoError(t, c.Save(context.Background(), arrow))
	require.NoError(t, c.Delete(context.Background(), arrow.Namespace))

	arrows, err := c.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, arrows)
}

func TestNewArrowCatalog_InvalidPath_ReturnsError(t *testing.T) {
	_, err := NewArrowCatalog("/invalid/path/arrows.db")
	assert.Error(t, err)
}

func TestArrowCatalog_Save_InnerError_ReturnsError(t *testing.T) {
	c := &arrowCatalog{inner: &errArrowStore{}}
	err := c.Save(context.Background(), makeTestArrow("github.com/org/repo", "Test"))
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

func TestArrowCatalog_Get_CorruptedManifest_ReturnsError(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	catalog := c.(*arrowCatalog)
	require.NoError(t, catalog.inner.Save(context.Background(), arrowRow{
		Namespace: "github.com/org/repo",
		Manifest:  "not-valid-json{{{",
		Removed:   false,
	}))

	_, err = c.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestArrowCatalog_List_CorruptedManifest_ReturnsError(t *testing.T) {
	c, err := NewArrowCatalog(":memory:")
	require.NoError(t, err)

	catalog := c.(*arrowCatalog)
	require.NoError(t, catalog.inner.Save(context.Background(), arrowRow{
		Namespace: "github.com/org/repo",
		Manifest:  "{invalid",
		Removed:   false,
	}))

	_, err = c.List(context.Background())
	assert.Error(t, err)
}
