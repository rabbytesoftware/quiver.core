package projections

import (
	"testing"

	"github.com/char2cs/asynx"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestInit_RegistersAllSubscriptions(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := asynx.New[domain.Quiver]().
		WithEventStore(es).
		Build()
	require.NoError(t, err)

	catalog := &mockQuiverCatalog{}

	err = Init(axQuiver, catalog)

	require.NoError(t, err)
}
