package sqlite

import (
	"os"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/adapter/store"
)

type testItem struct {
	ID    int    `db:"id"    json:"id"`
	Name  string `db:"name"  json:"name"`
	Count int    `db:"count" json:"count"`
	Flag  bool   `db:"flag"  json:"flag"`
}

func newTestStore(t *testing.T) store.Store[testItem, int] {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS test_items (
		id    INTEGER PRIMARY KEY,
		name  TEXT    NOT NULL,
		count INTEGER NOT NULL,
		flag  INTEGER NOT NULL
	)`)
	require.NoError(t, err)
	return New[testItem, int](db, "test_items", "id")
}

func TestSave_FindByKey(t *testing.T) {
	s := newTestStore(t)

	item := testItem{ID: 1, Name: "alice", Count: 42, Flag: true}
	err := s.Save(item)
	assert.NoError(t, err)

	found, err := s.FindByKey(1)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "alice", found.Name)
	assert.Equal(t, 42, found.Count)
	assert.Equal(t, true, found.Flag)
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)

	item := testItem{ID: 1, Name: "alice", Count: 42, Flag: false}
	err := s.Save(item)
	require.NoError(t, err)

	err = s.Delete(1)
	assert.NoError(t, err)

	found, err := s.FindByKey(1)
	assert.NoError(t, err)
	assert.Nil(t, found)
}

func TestFindAll(t *testing.T) {
	s := newTestStore(t)

	items := []testItem{
		{ID: 1, Name: "alice", Count: 10, Flag: true},
		{ID: 2, Name: "bob", Count: 20, Flag: false},
		{ID: 3, Name: "charlie", Count: 30, Flag: true},
	}

	for _, item := range items {
		err := s.Save(item)
		require.NoError(t, err)
	}

	found, err := s.FindAll()
	assert.NoError(t, err)
	assert.Len(t, found, 3)
}

func TestFindByKey_Missing(t *testing.T) {
	s := newTestStore(t)

	found, err := s.FindByKey(999)
	assert.NoError(t, err)
	assert.Nil(t, found)
}

func TestPersistentFile(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test_*.db")
	require.NoError(t, err)
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	openAndCreate := func() store.Store[testItem, int] {
		db, err := sqlx.Open("sqlite", tmpfile.Name())
		require.NoError(t, err)
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS test_items (id INTEGER PRIMARY KEY, name TEXT, count INTEGER, flag INTEGER)`)
		require.NoError(t, err)
		return New[testItem, int](db, "test_items", "id")
	}

	s1 := openAndCreate()
	require.NoError(t, s1.Save(testItem{ID: 1, Name: "alice", Count: 42, Flag: true}))

	s2 := openAndCreate()
	found, err := s2.FindByKey(1)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "alice", found.Name)
}

func TestFindAll_Empty(t *testing.T) {
	s := newTestStore(t)

	found, err := s.FindAll()
	assert.NoError(t, err)
	assert.Len(t, found, 0)
}

func TestSave_Update(t *testing.T) {
	s := newTestStore(t)

	item1 := testItem{ID: 1, Name: "alice", Count: 42, Flag: true}
	err := s.Save(item1)
	require.NoError(t, err)

	item2 := testItem{ID: 1, Name: "alice_updated", Count: 100, Flag: false}
	err = s.Save(item2)
	require.NoError(t, err)

	found, err := s.FindByKey(1)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "alice_updated", found.Name)
	assert.Equal(t, 100, found.Count)
	assert.Equal(t, false, found.Flag)
}

func TestGetColumnNames_NonStruct(t *testing.T) {
	cols, err := getColumnNames("not a struct")
	assert.Error(t, err)
	assert.Nil(t, cols)
}
