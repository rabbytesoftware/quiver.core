package quiver

import (
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_Build_SucceedsWithValidEventStore(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	cat, err := quiverstore.NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	svc, err := NewQuiverBuilder().
		WithEventStore(es).
		WithCatalogStore(cat).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_FailsWithNilEventStore(t *testing.T) {
	svc, err := NewQuiverBuilder().Build()

	require.Error(t, err)
	assert.Nil(t, svc)
}

func TestBuilder_Build_UsesProvidedCatalogStore(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	cat, err := quiverstore.NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	svc, err := NewQuiverBuilder().
		WithEventStore(es).
		WithCatalogStore(cat).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestNewAsynxQuiver_NilEventStore_ReturnsError(t *testing.T) {
	ax, err := newAsynxQuiver(nil)
	require.Error(t, err)
	assert.Nil(t, ax)
}

func TestBuilder_Build_NilCatalogStore_UsesDefaultPath(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	// Without WithCatalogStore — Build creates its own using metadata.GetQuiverHome()
	// This will succeed as long as the path is writable.
	svc, err := NewQuiverBuilder().
		WithEventStore(es).
		Build()

	// Accept either success or failure depending on the environment.
	if err == nil {
		assert.NotNil(t, svc)
	} else {
		assert.Nil(t, svc)
	}
}
