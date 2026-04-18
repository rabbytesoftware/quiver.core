package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) DepEdgeStore {
	t.Helper()
	st, err := NewDepEdgeStore(":memory:")
	require.NoError(t, err)
	return st
}

func row(fromNs, fromVer, toNs, toVer, constraint, depType string) DepEdgeRow {
	return DepEdgeRow{
		FromNamespace: fromNs,
		FromVersion:   fromVer,
		ToNamespace:   toNs,
		ToVersion:     toVer,
		Constraint:    constraint,
		DepType:       depType,
	}
}

func TestNewDepEdgeStore_memory(t *testing.T) {
	st, err := NewDepEdgeStore(":memory:")
	require.NoError(t, err)
	assert.NotNil(t, st)
}

func TestNewDepEdgeStore_invalidPath(t *testing.T) {
	// A path under a non-existent directory will fail to open.
	_, err := NewDepEdgeStore("/nonexistent-dir-quiver-test/db.sqlite")
	require.Error(t, err)
}

func TestSave_droppedTable(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	// Drop the dep_edges table so the Delete inside the transaction returns an error.
	err := st.(*depEdgeStore).db.Exec("DROP TABLE dep_edges").Error
	require.NoError(t, err)

	rows := []DepEdgeRow{
		row("github.com/org/app", "1.0.0", "github.com/org/lib", "1.0.0", "1.*", "tool"),
	}
	err = st.Save(ctx, "github.com/org/app", "1.0.0", rows)
	require.Error(t, err)
}

func TestSave_emptyRows_clearsScope(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	rows := []DepEdgeRow{
		row("github.com/org/app", "1.0.0", "github.com/org/lib", "1.0.0", "1.*", "tool"),
	}
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", rows))

	// Saving empty rows for the same scope should clear existing edges.
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", nil))

	result, err := st.ByDependency(ctx, "github.com/org/lib", "1.0.0")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestSave_and_ByDependency(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	rows := []DepEdgeRow{
		row("github.com/org/app", "1.0.0", "github.com/org/lib", "2.0.0", "2.*", "tool"),
	}
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", rows))

	result, err := st.ByDependency(ctx, "github.com/org/lib", "2.0.0")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "github.com/org/app", result[0].FromNamespace)
	assert.Equal(t, "1.0.0", result[0].FromVersion)
	assert.Equal(t, "2.*", result[0].Constraint)
	assert.Equal(t, "tool", result[0].DepType)
}

func TestSave_empty(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", nil))
}

func TestSave_idempotent(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	rows := []DepEdgeRow{
		row("github.com/org/app", "1.0.0", "github.com/org/lib", "2.0.0", "2.*", "tool"),
	}
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", rows))
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", rows))

	result, err := st.ByDependency(ctx, "github.com/org/lib", "2.0.0")
	require.NoError(t, err)
	assert.Len(t, result, 1)
}

func TestSave_replacesPreviousEdgesForSameFrom(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	rows1 := []DepEdgeRow{
		row("github.com/org/app", "1.0.0", "github.com/org/libA", "1.0.0", "1.*", "tool"),
	}
	rows2 := []DepEdgeRow{
		row("github.com/org/app", "1.0.0", "github.com/org/libB", "2.0.0", "2.*", "tool"),
	}
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", rows1))
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", rows2))

	old, err := st.ByDependency(ctx, "github.com/org/libA", "1.0.0")
	require.NoError(t, err)
	assert.Empty(t, old)

	newRows, err := st.ByDependency(ctx, "github.com/org/libB", "2.0.0")
	require.NoError(t, err)
	assert.Len(t, newRows, 1)
}

func TestDeleteFrom(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	rows := []DepEdgeRow{
		row("github.com/org/app", "1.0.0", "github.com/org/lib", "2.0.0", "2.*", "tool"),
	}
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", rows))
	require.NoError(t, st.DeleteFrom(ctx, "github.com/org/app", "1.0.0"))

	result, err := st.ByDependency(ctx, "github.com/org/lib", "2.0.0")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestDeleteFrom_nonexistent(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	err := st.DeleteFrom(ctx, "github.com/org/missing", "9.9.9")
	assert.NoError(t, err)
}

func TestHasAnyDependents_true(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	rows := []DepEdgeRow{
		row("github.com/org/app", "1.0.0", "github.com/org/lib", "1.0.0", "1.*", "tool"),
	}
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", rows))

	has, err := st.HasAnyDependents(ctx, "github.com/org/lib", "github.com/org/other")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasAnyDependents_excludes_from(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	rows := []DepEdgeRow{
		row("github.com/org/app", "1.0.0", "github.com/org/lib", "1.0.0", "1.*", "tool"),
	}
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", rows))

	has, err := st.HasAnyDependents(ctx, "github.com/org/lib", "github.com/org/app")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasAnyDependents_false_empty(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	has, err := st.HasAnyDependents(ctx, "github.com/org/lib", "github.com/org/app")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestByDependency_empty(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	result, err := st.ByDependency(ctx, "github.com/org/missing", "1.0.0")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestTableName(t *testing.T) {
	assert.Equal(t, "dep_edges", DepEdgeRow{}.TableName())
}

func TestSave_multipleRows(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	rows := []DepEdgeRow{
		row("github.com/org/app", "1.0.0", "github.com/org/libA", "1.0.0", "1.*", "tool"),
		row("github.com/org/app", "1.0.0", "github.com/org/libB", "2.0.0", "2.*", "service"),
	}
	require.NoError(t, st.Save(ctx, "github.com/org/app", "1.0.0", rows))

	resultA, err := st.ByDependency(ctx, "github.com/org/libA", "1.0.0")
	require.NoError(t, err)
	assert.Len(t, resultA, 1)

	resultB, err := st.ByDependency(ctx, "github.com/org/libB", "2.0.0")
	require.NoError(t, err)
	assert.Len(t, resultB, 1)
	assert.Equal(t, "service", resultB[0].DepType)
}

func TestHasAnyDependents_multipleDependents(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()

	rows1 := []DepEdgeRow{
		row("github.com/org/app1", "1.0.0", "github.com/org/lib", "1.0.0", "1.*", "tool"),
	}
	rows2 := []DepEdgeRow{
		row("github.com/org/app2", "2.0.0", "github.com/org/lib", "1.0.0", "1.*", "tool"),
	}
	require.NoError(t, st.Save(ctx, "github.com/org/app1", "1.0.0", rows1))
	require.NoError(t, st.Save(ctx, "github.com/org/app2", "2.0.0", rows2))

	has, err := st.HasAnyDependents(ctx, "github.com/org/lib", "github.com/org/app1")
	require.NoError(t, err)
	assert.True(t, has)
}
