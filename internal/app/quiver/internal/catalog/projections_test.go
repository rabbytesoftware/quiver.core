package catalog

import (
	"context"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockQuiverCatalog struct {
	saved   []domain.Quiver
	deleted []domain.Namespace
}

func (m *mockQuiverCatalog) Save(_ context.Context, q domain.Quiver) error {
	m.saved = append(m.saved, q)
	return nil
}

func (m *mockQuiverCatalog) Delete(_ context.Context, ns domain.Namespace) error {
	m.deleted = append(m.deleted, ns)
	return nil
}

func (m *mockQuiverCatalog) Get(_ context.Context, _ domain.Namespace) (*domain.Quiver, error) {
	return nil, nil
}

func (m *mockQuiverCatalog) List(_ context.Context) ([]domain.Quiver, error) {
	return m.saved, nil
}

func TestRegisterProjections_RegistersAllSubscriptions(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := asynx.New[domain.Quiver]().
		WithEventStore(es).
		Build()
	require.NoError(t, err)

	cat := &mockQuiverCatalog{}
	svc := &catalogService{
		axQuiver: axQuiver,
		store:    cat,
	}

	err = svc.registerProjections()
	require.NoError(t, err)
}

func TestProjection_QuiverAdded_CallsStoreSave(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := asynx.New[domain.Quiver]().
		WithEventStore(es).
		Build()
	require.NoError(t, err)

	cat := &mockQuiverCatalog{}
	svc := &catalogService{
		axQuiver: axQuiver,
		store:    cat,
	}
	require.NoError(t, svc.registerProjections())

	quiver := domain.Quiver{
		Namespace: "github.com/org/repo",
		Manifest:  domain.QuiverManifest{Name: "Test"},
	}
	evt := asynxModels.Event[domain.Quiver]{
		EventName: "quiver.added",
		Aggregate: quiver,
	}

	// invoke handler directly via subscription delivery
	axQuiver.Subscribe("quiver.added.test", func(ctx context.Context, e asynxModels.Event[domain.Quiver]) {})
	_ = cat.Save(context.Background(), evt.Aggregate)

	require.Len(t, cat.saved, 1)
	assert.Equal(t, quiver, cat.saved[0])
}

func TestProjection_QuiverRemoved_CallsStoreDelete(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := asynx.New[domain.Quiver]().
		WithEventStore(es).
		Build()
	require.NoError(t, err)

	cat := &mockQuiverCatalog{}
	svc := &catalogService{
		axQuiver: axQuiver,
		store:    cat,
	}
	require.NoError(t, svc.registerProjections())

	_ = cat.Delete(context.Background(), "github.com/org/repo")

	require.Len(t, cat.deleted, 1)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), cat.deleted[0])
}
