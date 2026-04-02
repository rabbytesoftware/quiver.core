package sqlite

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testItem struct {
	ID    int    `db:"id"    json:"id"`
	Name  string `db:"name"  json:"name"`
	Count int    `db:"count" json:"count"`
	Flag  bool   `db:"flag"  json:"flag"`
}

func TestNew(t *testing.T) {
	s, err := New[testItem](":memory:", "test_items", "id")
	require.NoError(t, err)
	assert.NotNil(t, s)
}

func TestSave_FindByID(t *testing.T) {
	s, err := New[testItem](":memory:", "test_items", "id")
	require.NoError(t, err)

	item := testItem{ID: 1, Name: "alice", Count: 42, Flag: true}
	err = s.Save(item)
	assert.NoError(t, err)

	found, err := s.FindByID(1)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "alice", found.Name)
	assert.Equal(t, 42, found.Count)
	assert.Equal(t, true, found.Flag)
}

func TestDelete(t *testing.T) {
	s, err := New[testItem](":memory:", "test_items", "id")
	require.NoError(t, err)

	item := testItem{ID: 1, Name: "alice", Count: 42, Flag: false}
	err = s.Save(item)
	require.NoError(t, err)

	err = s.Delete(1)
	assert.NoError(t, err)

	found, err := s.FindByID(1)
	assert.NoError(t, err)
	assert.Nil(t, found)
}

func TestFindAll(t *testing.T) {
	s, err := New[testItem](":memory:", "test_items", "id")
	require.NoError(t, err)

	items := []testItem{
		{ID: 1, Name: "alice", Count: 10, Flag: true},
		{ID: 2, Name: "bob", Count: 20, Flag: false},
		{ID: 3, Name: "charlie", Count: 30, Flag: true},
	}

	for _, item := range items {
		err = s.Save(item)
		require.NoError(t, err)
	}

	found, err := s.FindAll()
	assert.NoError(t, err)
	assert.Len(t, found, 3)
}

func TestFindByID_Missing(t *testing.T) {
	s, err := New[testItem](":memory:", "test_items", "id")
	require.NoError(t, err)

	found, err := s.FindByID(999)
	assert.NoError(t, err)
	assert.Nil(t, found)
}

func TestPersistentFile(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test_*.db")
	require.NoError(t, err)
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	// Create and save
	s1, err := New[testItem](tmpfile.Name(), "test_items", "id")
	require.NoError(t, err)

	item := testItem{ID: 1, Name: "alice", Count: 42, Flag: true}
	err = s1.Save(item)
	require.NoError(t, err)

	// Reopen and verify
	s2, err := New[testItem](tmpfile.Name(), "test_items", "id")
	require.NoError(t, err)

	found, err := s2.FindByID(1)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "alice", found.Name)
}
