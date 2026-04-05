package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testItem struct {
	ID    int
	Name  string
	Value int
}

func TestMemory_Save_FindByKey(
	t *testing.T,
) {
	store := NewMemory[testItem, int](func(ti testItem) int { return ti.ID })
	item := testItem{ID: 1, Name: "test", Value: 42}

	err := store.Save(item)
	require.NoError(t, err)

	found, err := store.FindByKey(1)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, item, *found)
}

func TestMemory_FindByKey_NotFound(
	t *testing.T,
) {
	store := NewMemory[testItem, int](func(ti testItem) int { return ti.ID })

	found, err := store.FindByKey(999)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestMemory_Delete(
	t *testing.T,
) {
	store := NewMemory[testItem, int](func(ti testItem) int { return ti.ID })
	item := testItem{ID: 1, Name: "test"}

	require.NoError(t, store.Save(item))
	require.NoError(t, store.Delete(1))

	found, err := store.FindByKey(1)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestMemory_FindAll(
	t *testing.T,
) {
	store := NewMemory[testItem, int](func(ti testItem) int { return ti.ID })
	item1 := testItem{ID: 1, Name: "first"}
	item2 := testItem{ID: 2, Name: "second"}

	require.NoError(t, store.Save(item1))
	require.NoError(t, store.Save(item2))

	results, err := store.FindAll()
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestMemory_Save_Overwrite(
	t *testing.T,
) {
	store := NewMemory[testItem, int](func(ti testItem) int { return ti.ID })
	item1 := testItem{ID: 1, Name: "first"}
	item2 := testItem{ID: 1, Name: "second"}

	require.NoError(t, store.Save(item1))
	require.NoError(t, store.Save(item2))

	found, err := store.FindByKey(1)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, item2, *found)
}
