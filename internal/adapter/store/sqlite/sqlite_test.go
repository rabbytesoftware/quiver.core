package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testItem struct {
	ID   int    `gorm:"primaryKey"`
	Name string `gorm:"not null"`
}

func (testItem) TableName() string { return "test_items" }

func newTestStore(t *testing.T) *gormStore[testItem, int] {
	t.Helper()
	s, err := New[testItem, int](":memory:")
	require.NoError(t, err)
	return s.(*gormStore[testItem, int])
}

func TestNew_InMemory(t *testing.T) {
	s, err := New[testItem, int](":memory:")
	require.NoError(t, err)
	assert.NotNil(t, s)
}

func TestSave_FindByKey(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.Save(context.Background(), testItem{ID: 1, Name: "alice"}))

	found, err := s.FindByKey(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "alice", found.Name)
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.Save(context.Background(), testItem{ID: 1, Name: "alice"}))
	require.NoError(t, s.Delete(context.Background(), 1))

	found, err := s.FindByKey(context.Background(), 1)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestFindAll(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.Save(context.Background(), testItem{ID: 1, Name: "alice"}))
	require.NoError(t, s.Save(context.Background(), testItem{ID: 2, Name: "bob"}))

	all, err := s.FindAll(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestFindByKey_Missing(t *testing.T) {
	s := newTestStore(t)

	found, err := s.FindByKey(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestSave_Upsert(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.Save(context.Background(), testItem{ID: 1, Name: "alice"}))
	require.NoError(t, s.Save(context.Background(), testItem{ID: 1, Name: "alice_updated"}))

	found, err := s.FindByKey(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "alice_updated", found.Name)
}

func TestNew_InvalidPath_ReturnsError(t *testing.T) {
	_, err := New[testItem, int]("/invalid/path/that/does/not/exist/db.db")
	assert.Error(t, err)
}

type noPKItem struct {
	Code string `gorm:"column:code"`
	Name string `gorm:"column:name"`
}

func (noPKItem) TableName() string { return "no_pk_items" }

func TestNew_NoPrimaryKey_ReturnsError(t *testing.T) {
	_, err := New[noPKItem, int](":memory:")
	assert.Error(t, err)
}

func TestFindByKey_DBClosed_ReturnsError(t *testing.T) {
	s, err := New[testItem, int](":memory:")
	require.NoError(t, err)

	sqlDB, err := s.(*gormStore[testItem, int]).db.DB()
	require.NoError(t, err)
	sqlDB.Close()

	_, err = s.FindByKey(context.Background(), 1)
	assert.Error(t, err)
}

func TestFindAll_DBClosed_ReturnsError(t *testing.T) {
	s, err := New[testItem, int](":memory:")
	require.NoError(t, err)

	sqlDB, err := s.(*gormStore[testItem, int]).db.DB()
	require.NoError(t, err)
	sqlDB.Close()

	_, err = s.FindAll(context.Background())
	assert.Error(t, err)
}

func TestOpenDB_Memory_ReturnsDB(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)
	assert.NotNil(t, db)
}

func TestOpenDB_InvalidPath_ReturnsError(t *testing.T) {
	_, err := OpenDB("/nonexistent-dir-quiver/db.sqlite")
	require.Error(t, err)
}

type testRow struct {
	ID   string `gorm:"primaryKey;column:id"`
	Name string `gorm:"not null;column:name"`
}

func (testRow) TableName() string { return "test_rows" }

func TestNewFromDB_NoPrimaryKey_ReturnsError(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)

	_, err = NewFromDB[noPKItem, int](db)
	assert.Error(t, err)
}

func TestPrimaryKeyColumn_NoPK_ReturnsError(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)

	_, err = primaryKeyColumn[noPKItem](db)
	assert.Error(t, err)
}

func TestNewFromDB_ReturnsWorkingStore(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)

	st, err := NewFromDB[testRow, string](db)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, st.Save(ctx, testRow{ID: "a", Name: "hello"}))

	got, err := st.FindByKey(ctx, "a")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "hello", got.Name)
}

func TestNew_FailedMigration_ReturnsError(t *testing.T) {
	// noPKItem has no primary key — will fail at primaryKeyColumn
	_, err := New[noPKItem, int](":memory:")
	assert.Error(t, err)
}

func TestNewFromDB_SuccessfulMigration(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)

	st, err := NewFromDB[testItem, int](db)
	require.NoError(t, err)
	assert.NotNil(t, st)
}
