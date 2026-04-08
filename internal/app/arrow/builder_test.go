package arrow

import (
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) arrowstore.ArrowCatalog {
	t.Helper()
	store, err := arrowstore.NewArrowCatalog(":memory:")
	require.NoError(t, err)
	return store
}

func TestBuilder_Build_SucceedsWithValidEventStore(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	svc, err := NewArrowBuilder().
		WithEventStore(es).
		WithCatalogStore(newTestStore(t)).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_FailsWithNilEventStore(t *testing.T) {
	svc, err := NewArrowBuilder().Build()

	require.Error(t, err)
	assert.Nil(t, svc)
}

func TestBuilder_Build_UsesProvidedCatalogStore(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	svc, err := NewArrowBuilder().
		WithEventStore(es).
		WithCatalogStore(newTestStore(t)).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_UsesProvidedCatalog(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	// Build a standalone catalog using its own event stores.
	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)
	cat, err := catalog.New(axArrow, axRuntime, newTestStore(t), nil, nil)
	require.NoError(t, err)

	svc, err := NewArrowBuilder().
		WithEventStore(es).
		WithCatalog(cat).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}
