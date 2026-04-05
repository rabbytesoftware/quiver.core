package projections

import (
	"testing"

	"github.com/char2cs/asynx"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
	"github.com/stretchr/testify/require"
)

func TestInit_RegistersAllSubscriptions(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		Build()
	require.NoError(t, err)

	axRuntime, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		Build()
	require.NoError(t, err)

	catalog := &mockArrowCatalog{}
	var wiz wizard.Wizard

	err = Init(axArrow, axRuntime, catalog, wiz)

	require.NoError(t, err)
}
