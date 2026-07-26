package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

var errStoreFailure = errors.New("store failure")

type errQuiverStore struct{}

func (e *errQuiverStore) Save(_ context.Context, _ collectionRow) error { return errStoreFailure }
func (e *errQuiverStore) Delete(_ context.Context, _ string) error      { return errStoreFailure }
func (e *errQuiverStore) FindByKey(_ context.Context, _ string) (*collectionRow, error) {
	return nil, errStoreFailure
}

func (e *errQuiverStore) FindAll(_ context.Context) ([]collectionRow, error) {
	return nil, errStoreFailure
}

func (e *errQuiverStore) Close() error { return errStoreFailure }

func makeTestQuiver(ns string) domain.Collection {
	return domain.Collection{
		Namespace:  domain.Namespace(ns),
		FollowedAt: time.Now().Truncate(time.Second),
	}
}

func TestQuiverStore_SaveAndGet_ReturnsSavedQuiver(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo")

	require.NoError(t, s.Save(context.Background(), quiver))

	got, err := s.Get(context.Background(), quiver.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, quiver.Namespace, got.Namespace)
	assert.Equal(t, quiver.FollowedAt.Unix(), got.FollowedAt.Unix())
}

func TestQuiverStore_SaveDeleteGet_ReturnsNil(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	quiver := makeTestQuiver("github.com/org/repo")

	require.NoError(t, s.Save(context.Background(), quiver))
	require.NoError(t, s.Delete(context.Background(), quiver.Namespace))

	got, err := s.Get(context.Background(), quiver.Namespace)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestQuiverStore_List_ReturnsAllSaved(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	q1 := makeTestQuiver("github.com/org/repo1")
	q2 := makeTestQuiver("github.com/org/repo2")

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

	quiver := makeTestQuiver("github.com/org/repo")

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

	quiver := makeTestQuiver("github.com/org/repo")
	require.NoError(t, s.Save(context.Background(), quiver))

	quiver.FollowedAt = quiver.FollowedAt.Add(time.Hour)
	require.NoError(t, s.Save(context.Background(), quiver))

	got, err := s.Get(context.Background(), quiver.Namespace)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, quiver.FollowedAt.Unix(), got.FollowedAt.Unix())
}

func TestNew_InvalidPath_ReturnsError(t *testing.T) {
	_, err := New("/invalid/path/quivers.db")
	assert.Error(t, err)
}

func TestQuiverStore_Save_InnerError_ReturnsError(t *testing.T) {
	s := &collectionStore{inner: &errQuiverStore{}}
	err := s.Save(context.Background(), makeTestQuiver("github.com/org/repo"))
	assert.Error(t, err)
}

func TestQuiverStore_Get_InnerError_ReturnsError(t *testing.T) {
	s := &collectionStore{inner: &errQuiverStore{}}
	_, err := s.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
}

func TestQuiverStore_List_InnerError_ReturnsError(t *testing.T) {
	s := &collectionStore{inner: &errQuiverStore{}}
	_, err := s.List(context.Background())
	assert.Error(t, err)
}

func TestQuiverStore_Close_ReleasesHandle(t *testing.T) {
	s, err := New(":memory:")
	require.NoError(t, err)

	require.NoError(t, s.Close())

	_, err = s.List(context.Background())
	assert.Error(t, err, "a closed collections database must refuse further reads")
}

func TestQuiverStore_Close_InnerError_ReturnsError(t *testing.T) {
	s := &collectionStore{inner: &errQuiverStore{}}

	err := s.Close()

	require.Error(t, err)
	assert.ErrorIs(t, err, errStoreFailure)
}
