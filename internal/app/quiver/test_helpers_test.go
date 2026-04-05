package quiver

import (
	"context"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	quivercmds "github.com/rabbytesoftware/quiver/internal/app/quiver/commands"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/require"
)

func makeTestManifest(name string) *domain.QuiverManifest {
	return &domain.QuiverManifest{
		Name:        name,
		Description: "A test quiver",
		Tags:        []string{"test"},
	}
}

func makeQuiverServiceWithMocks(v *mocks.Vault, m *mocks.Manifold) *quiverService {
	return &quiverService{
		vault:    v,
		manifold: m,
	}
}

func testQuiverService(t *testing.T, v *mocks.Vault, m *mocks.Manifold) *quiverService {
	t.Helper()

	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	catalog, err := quiverstore.NewQuiverCatalogFromPath(":memory:")
	require.NoError(t, err)

	return &quiverService{
		asynxQuiver: axQuiver,
		catalog:     catalog,
		vault:       v,
		manifold:    m,
	}
}

func addQuiverForTest(t *testing.T, svc *quiverService, ns domain.Namespace, manifest *domain.QuiverManifest) {
	t.Helper()
	require.NoError(t, svc.asynxQuiver.Send(context.Background(), quivercmds.AddQuiver{
		Namespace: ns,
		Manifest:  *manifest,
	}))
	svc.asynxQuiver.WaitPublish()
}
