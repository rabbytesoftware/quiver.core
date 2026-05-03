package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterSQLite "github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/graph/internal/store"
)

func newTestStore(t *testing.T) store.DepEdgeStore {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	es, err := store.NewDepEdgeStore(db)
	require.NoError(t, err)
	return es
}

func TestNewDepEdgeStore_NilDB(t *testing.T) {
	_, err := store.NewDepEdgeStore(nil)
	require.Error(t, err)
}

func TestSave_AndFindByDependency(t *testing.T) {
	s := newTestStore(t)
	rows := []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/pkg",
			FromVersion:   "v1.0.0",
			ToNamespace:   "github.com/user/dep",
			ToVersion:     "v1.0.0",
			Constraint:    "^v1",
			DepType:       "tool",
		},
	}

	err := s.Save(context.Background(), "github.com/user/pkg", "v1.0.0", rows)
	require.NoError(t, err)

	found, err := s.ByDependency(context.Background(), "github.com/user/dep", "v1.0.0")
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, "github.com/user/pkg", found[0].FromNamespace)
}

func TestSave_ReplacesExistingRows(t *testing.T) {
	s := newTestStore(t)
	initial := []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/pkg",
			FromVersion:   "v1.0.0",
			ToNamespace:   "github.com/user/dep-old",
			ToVersion:     "v1.0.0",
			Constraint:    "",
			DepType:       "tool",
		},
	}
	err := s.Save(context.Background(), "github.com/user/pkg", "v1.0.0", initial)
	require.NoError(t, err)

	updated := []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/pkg",
			FromVersion:   "v1.0.0",
			ToNamespace:   "github.com/user/dep-new",
			ToVersion:     "v1.0.0",
			Constraint:    "",
			DepType:       "tool",
		},
	}
	err = s.Save(context.Background(), "github.com/user/pkg", "v1.0.0", updated)
	require.NoError(t, err)

	// Old dep should be gone
	old, err := s.ByDependency(context.Background(), "github.com/user/dep-old", "v1.0.0")
	require.NoError(t, err)
	assert.Empty(t, old)

	// New dep should be present
	newDep, err := s.ByDependency(context.Background(), "github.com/user/dep-new", "v1.0.0")
	require.NoError(t, err)
	assert.Len(t, newDep, 1)
}

func TestSave_EmptyRows_ClearsExisting(t *testing.T) {
	s := newTestStore(t)
	rows := []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/pkg",
			FromVersion:   "v1.0.0",
			ToNamespace:   "github.com/user/dep",
			ToVersion:     "v1.0.0",
			Constraint:    "",
			DepType:       "tool",
		},
	}
	err := s.Save(context.Background(), "github.com/user/pkg", "v1.0.0", rows)
	require.NoError(t, err)

	err = s.Save(context.Background(), "github.com/user/pkg", "v1.0.0", nil)
	require.NoError(t, err)

	found, err := s.ByDependency(context.Background(), "github.com/user/dep", "v1.0.0")
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestDeleteFrom(t *testing.T) {
	s := newTestStore(t)
	rows := []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/pkg",
			FromVersion:   "v1.0.0",
			ToNamespace:   "github.com/user/dep",
			ToVersion:     "v1.0.0",
			Constraint:    "",
			DepType:       "tool",
		},
	}
	err := s.Save(context.Background(), "github.com/user/pkg", "v1.0.0", rows)
	require.NoError(t, err)

	err = s.DeleteFrom(context.Background(), "github.com/user/pkg", "v1.0.0")
	require.NoError(t, err)

	found, err := s.ByDependency(context.Background(), "github.com/user/dep", "v1.0.0")
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestEdgesToBare(t *testing.T) {
	s := newTestStore(t)
	rows := []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/pkg",
			FromVersion:   "v1.0.0",
			ToNamespace:   "github.com/user/dep",
			ToVersion:     "v1.0.0",
			Constraint:    "",
			DepType:       "tool",
		},
		{
			FromNamespace: "github.com/user/other",
			FromVersion:   "v2.0.0",
			ToNamespace:   "github.com/user/dep",
			ToVersion:     "v2.0.0",
			Constraint:    "",
			DepType:       "tool",
		},
	}
	err := s.Save(context.Background(), "github.com/user/pkg", "v1.0.0", rows[:1])
	require.NoError(t, err)
	err = s.Save(context.Background(), "github.com/user/other", "v2.0.0", rows[1:])
	require.NoError(t, err)

	found, err := s.EdgesToBare(context.Background(), "github.com/user/dep")
	require.NoError(t, err)
	assert.Len(t, found, 2)
}

func TestHasAnyDependents_True(t *testing.T) {
	s := newTestStore(t)
	rows := []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/parent",
			FromVersion:   "v1.0.0",
			ToNamespace:   "github.com/user/dep",
			ToVersion:     "v1.0.0",
			Constraint:    "",
			DepType:       "tool",
		},
	}
	err := s.Save(context.Background(), "github.com/user/parent", "v1.0.0", rows)
	require.NoError(t, err)

	has, err := s.HasAnyDependents(context.Background(), "github.com/user/dep", "")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasAnyDependents_False_WhenExcluded(t *testing.T) {
	s := newTestStore(t)
	rows := []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/parent",
			FromVersion:   "v1.0.0",
			ToNamespace:   "github.com/user/dep",
			ToVersion:     "v1.0.0",
			Constraint:    "",
			DepType:       "tool",
		},
	}
	err := s.Save(context.Background(), "github.com/user/parent", "v1.0.0", rows)
	require.NoError(t, err)

	// Exclude the only parent
	has, err := s.HasAnyDependents(context.Background(), "github.com/user/dep", "github.com/user/parent")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasAnyDependents_False_NoRows(t *testing.T) {
	s := newTestStore(t)

	has, err := s.HasAnyDependents(context.Background(), "github.com/user/nobody", "")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestByDependency_NoRows(t *testing.T) {
	s := newTestStore(t)

	found, err := s.ByDependency(context.Background(), "github.com/user/dep", "v1.0.0")
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestDeleteTo(t *testing.T) {
	s := newTestStore(t)

	edgesA := []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/a",
			FromVersion:   "v1.0.0",
			ToNamespace:   "github.com/user/shared",
			ToVersion:     "v1.0.0",
			Constraint:    "",
			DepType:       "tool",
		},
	}
	edgesC := []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/c",
			FromVersion:   "v1.0.0",
			ToNamespace:   "github.com/user/shared",
			ToVersion:     "v1.0.0",
			Constraint:    "",
			DepType:       "tool",
		},
	}
	err := s.Save(context.Background(), "github.com/user/a", "v1.0.0", edgesA)
	require.NoError(t, err)
	err = s.Save(context.Background(), "github.com/user/c", "v1.0.0", edgesC)
	require.NoError(t, err)

	err = s.DeleteTo(context.Background(), "github.com/user/shared")
	require.NoError(t, err)

	found, err := s.ByDependency(context.Background(), "github.com/user/shared", "v1.0.0")
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestNewDepEdgeStore_ClosedDB_Error(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = store.NewDepEdgeStore(db)
	require.Error(t, err)
}

func TestSave_ClosedDB_Error(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	s, storeErr := store.NewDepEdgeStore(db)
	require.NoError(t, storeErr)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = s.Save(context.Background(), "github.com/user/pkg", "v1.0.0", nil)
	require.Error(t, err)
}
