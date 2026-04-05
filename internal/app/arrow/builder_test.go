package arrow

import (
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_Build_SucceedsWithValidEventStore(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	catalog, err := arrowstore.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	svc, err := NewArrowBuilder().
		WithEventStore(es).
		WithCatalog(catalog).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_FailsWithNilEventStore(t *testing.T) {
	svc, err := NewArrowBuilder().Build()

	require.Error(t, err)
	assert.Nil(t, svc)
}

func TestBuilder_Build_UsesProvidedCatalog(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	catalog, err := arrowstore.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	svc, err := NewArrowBuilder().
		WithEventStore(es).
		WithCatalog(catalog).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}
