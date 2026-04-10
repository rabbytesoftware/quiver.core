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

type errQuiverStore struct{}

func (e *errQuiverStore) Save(_ context.Context, _ quiverRow) error { return errStoreFailure }
func (e *errQuiverStore) Delete(_ context.Context, _ string) error  { return errStoreFailure }
func (e *errQuiverStore) FindByKey(_ context.Context, _ string) (*quiverRow, error) {
	return nil, errStoreFailure
}
func (e *errQuiverStore) FindAll(_ context.Context) ([]quiverRow, error) {
	return nil, errStoreFailure
}

func makeTestQuiver(ns string, name string) domain.Quiver {
	return domain.Quiver{
		Namespace: domain.Namespace(ns),
		Manifest: domain.QuiverManifest{
			Name:        name,
			Description: "A test quiver",
			Tags:        []string{"test"},
		},
		Removed: false,
	}
}

func TestQuiverCatalog_SaveAndGet_ReturnsSavedQuiver(t *testing.T) {
	c, err := NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")

	err = c.Save(context.Background(), quiver)
	require.NoError(t, err)

	got, err := c.Get(context.Background(), quiver.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, quiver.Namespace, got.Namespace)
	assert.Equal(t, quiver.Manifest.Name, got.Manifest.Name)
	assert.Equal(t, quiver.Manifest.Description, got.Manifest.Description)
	assert.Equal(t, quiver.Removed, got.Removed)
}

func TestQuiverCatalog_SaveDeleteGet_ReturnsNil(t *testing.T) {
	c, err := NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")

	err = c.Save(context.Background(), quiver)
	require.NoError(t, err)

	err = c.Delete(context.Background(), quiver.Namespace)
	require.NoError(t, err)

	got, err := c.Get(context.Background(), quiver.Namespace)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestQuiverCatalog_List_ReturnsAllSaved(t *testing.T) {
	c, err := NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	q1 := makeTestQuiver("github.com/org/repo1", "Quiver1")
	q2 := makeTestQuiver("github.com/org/repo2", "Quiver2")

	require.NoError(t, c.Save(context.Background(), q1))
	require.NoError(t, c.Save(context.Background(), q2))

	quivers, err := c.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, quivers, 2)

	namespaces := make([]domain.Namespace, 0, len(quivers))
	for _, q := range quivers {
		namespaces = append(namespaces, q.Namespace)
	}
	assert.Contains(t, namespaces, q1.Namespace)
	assert.Contains(t, namespaces, q2.Namespace)
}

func TestQuiverCatalog_ListAfterRemove_ReturnsEmpty(t *testing.T) {
	c, err := NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")

	require.NoError(t, c.Save(context.Background(), quiver))
	require.NoError(t, c.Delete(context.Background(), quiver.Namespace))

	quivers, err := c.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, quivers)
}

func TestQuiverCatalog_GetNonExistent_ReturnsNil(t *testing.T) {
	c, err := NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	got, err := c.Get(context.Background(), "github.com/org/nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestQuiverCatalog_Save_UpdatesExisting(t *testing.T) {
	c, err := NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")
	require.NoError(t, c.Save(context.Background(), quiver))

	quiver.Manifest.Name = "UpdatedQuiver"
	require.NoError(t, c.Save(context.Background(), quiver))

	got, err := c.Get(context.Background(), quiver.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "UpdatedQuiver", got.Manifest.Name)
}

func TestNewQuiverCatalog_InvalidPath_ReturnsError(t *testing.T) {
	_, err := NewQuiverCatalog("/invalid/path/quivers.db")
	assert.Error(t, err)
}

func TestQuiverCatalog_Save_InnerError_ReturnsError(t *testing.T) {
	c := &quiverCatalog{inner: &errQuiverStore{}}
	err := c.Save(context.Background(), makeTestQuiver("github.com/org/repo", "Test"))
	assert.Error(t, err)
}

func TestQuiverCatalog_Get_InnerError_ReturnsError(t *testing.T) {
	c := &quiverCatalog{inner: &errQuiverStore{}}
	_, err := c.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestQuiverCatalog_List_InnerError_ReturnsError(t *testing.T) {
	c := &quiverCatalog{inner: &errQuiverStore{}}
	_, err := c.List(context.Background())
	assert.Error(t, err)
}

func TestQuiverCatalog_Get_CorruptedManifest_ReturnsError(t *testing.T) {
	c, err := NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	catalog := c.(*quiverCatalog)
	require.NoError(t, catalog.inner.Save(context.Background(), quiverRow{
		Namespace: "github.com/org/repo",
		Manifest:  "not-valid-json{{{",
		Removed:   false,
	}))

	_, err = c.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestQuiverCatalog_List_CorruptedManifest_ReturnsError(t *testing.T) {
	c, err := NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	catalog := c.(*quiverCatalog)
	require.NoError(t, catalog.inner.Save(context.Background(), quiverRow{
		Namespace: "github.com/org/repo",
		Manifest:  "{invalid",
		Removed:   false,
	}))

	_, err = c.List(context.Background())
	assert.Error(t, err)
}
