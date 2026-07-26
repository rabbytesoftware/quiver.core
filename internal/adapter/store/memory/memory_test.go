package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testItem struct {
	ID   int
	Name string
}

func TestMemory_SaveAndFindByKey(t *testing.T) {
	s := NewMemory[testItem, int](func(i testItem) int { return i.ID })

	require.NoError(t, s.Save(context.Background(), testItem{ID: 1, Name: "alice"}))

	found, err := s.FindByKey(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "alice", found.Name)
}

func TestMemory_Delete(t *testing.T) {
	s := NewMemory[testItem, int](func(i testItem) int { return i.ID })

	require.NoError(t, s.Save(context.Background(), testItem{ID: 1, Name: "alice"}))
	require.NoError(t, s.Delete(context.Background(), 1))

	found, err := s.FindByKey(context.Background(), 1)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestMemory_FindAll(t *testing.T) {
	s := NewMemory[testItem, int](func(i testItem) int { return i.ID })

	require.NoError(t, s.Save(context.Background(), testItem{ID: 1, Name: "alice"}))
	require.NoError(t, s.Save(context.Background(), testItem{ID: 2, Name: "bob"}))

	all, err := s.FindAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestMemory_FindByKey_Missing(t *testing.T) {
	s := NewMemory[testItem, int](func(i testItem) int { return i.ID })

	found, err := s.FindByKey(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestMemory_Close_ReturnsNilAndKeepsDataReadable(t *testing.T) {
	s := NewMemory[testItem, int](func(i testItem) int { return i.ID })
	require.NoError(t, s.Save(context.Background(), testItem{ID: 1, Name: "alice"}))

	require.NoError(t, s.Close())

	all, err := s.FindAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 1)
}
