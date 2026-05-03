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
	}
}

func TestQuiverStore_SaveAndGet_ReturnsSavedQuiver(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")

	require.NoError(t, s.Save(context.Background(), quiver))

	got, err := s.Get(context.Background(), quiver.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, quiver.Namespace, got.Namespace)
	assert.Equal(t, quiver.Manifest.Name, got.Manifest.Name)
	assert.Equal(t, quiver.Manifest.Description, got.Manifest.Description)
}

func TestQuiverStore_SaveDeleteGet_ReturnsNil(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")

	require.NoError(t, s.Save(context.Background(), quiver))
	require.NoError(t, s.Delete(context.Background(), quiver.Namespace))

	got, err := s.Get(context.Background(), quiver.Namespace)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestQuiverStore_List_ReturnsAllSaved(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	q1 := makeTestQuiver("github.com/org/repo1", "Quiver1")
	q2 := makeTestQuiver("github.com/org/repo2", "Quiver2")

	require.NoError(t, s.Save(context.Background(), q1))
	require.NoError(t, s.Save(context.Background(), q2))

	quivers, err := s.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, quivers, 2)

	namespaces := make([]domain.Namespace, 0, len(quivers))
	for _, q := range quivers {
		namespaces = append(namespaces, q.Namespace)
	}
	assert.Contains(t, namespaces, q1.Namespace)
	assert.Contains(t, namespaces, q2.Namespace)
}

func TestQuiverStore_ListAfterRemove_ReturnsEmpty(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")

	require.NoError(t, s.Save(context.Background(), quiver))
	require.NoError(t, s.Delete(context.Background(), quiver.Namespace))

	quivers, err := s.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, quivers)
}

func TestQuiverStore_GetNonExistent_ReturnsNil(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	got, err := s.Get(context.Background(), "github.com/org/nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestQuiverStore_Save_UpdatesExisting(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo", "MyQuiver")
	require.NoError(t, s.Save(context.Background(), quiver))

	quiver.Manifest.Name = "UpdatedQuiver"
	require.NoError(t, s.Save(context.Background(), quiver))

	got, err := s.Get(context.Background(), quiver.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "UpdatedQuiver", got.Manifest.Name)
}

func TestNew_InvalidPath_ReturnsError(t *testing.T) {
	_, err := New("/invalid/path/quivers.db")
	assert.Error(t, err)
}

func TestQuiverStore_Save_InnerError_ReturnsError(t *testing.T) {
	s := &quiverStore{inner: &errQuiverStore{}}
	err := s.Save(context.Background(), makeTestQuiver("github.com/org/repo", "Test"))
	assert.Error(t, err)
}

func TestQuiverStore_Get_InnerError_ReturnsError(t *testing.T) {
	s := &quiverStore{inner: &errQuiverStore{}}
	_, err := s.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestQuiverStore_List_InnerError_ReturnsError(t *testing.T) {
	s := &quiverStore{inner: &errQuiverStore{}}
	_, err := s.List(context.Background())
	assert.Error(t, err)
}

func TestQuiverStore_Get_CorruptedManifest_ReturnsError(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	catalog := s.(*quiverStore)
	require.NoError(t, catalog.inner.Save(context.Background(), quiverRow{
		Namespace: "github.com/org/repo",
		Manifest:  "not-valid-json{{{",
	}))

	_, err = s.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestQuiverStore_List_CorruptedManifest_ReturnsError(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	catalog := s.(*quiverStore)
	require.NoError(t, catalog.inner.Save(context.Background(), quiverRow{
		Namespace: "github.com/org/repo",
		Manifest:  "{invalid",
	}))

	_, err = s.List(context.Background())
	assert.Error(t, err)
}
