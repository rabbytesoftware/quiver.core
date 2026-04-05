package quiver

import (
	"context"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_Build_SucceedsWithValidEventStore(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	catalog, err := quiverstore.NewQuiverCatalogFromPath(":memory:")
	require.NoError(t, err)

	svc, err := NewQuiverBuilder().
		WithEventStore(es).
		WithCatalog(catalog).
		Build(context.Background())

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_FailsWithNilEventStore(t *testing.T) {
	svc, err := NewQuiverBuilder().Build(context.Background())

	require.Error(t, err)
	assert.Nil(t, svc)
}

func TestBuilder_Build_UsesProvidedCatalog(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	catalog, err := quiverstore.NewQuiverCatalogFromPath(":memory:")
	require.NoError(t, err)

	svc, err := NewQuiverBuilder().
		WithEventStore(es).
		WithCatalog(catalog).
		Build(context.Background())

	require.NoError(t, err)
	assert.NotNil(t, svc)
}
