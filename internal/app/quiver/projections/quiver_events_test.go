package projections

import (
	"context"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockQuiverCatalog struct {
	saved   []domain.Quiver
	deleted []domain.Namespace
}

func (m *mockQuiverCatalog) Save(q domain.Quiver) error {
	m.saved = append(m.saved, q)
	return nil
}

func (m *mockQuiverCatalog) Delete(ns domain.Namespace) error {
	m.deleted = append(m.deleted, ns)
	return nil
}

func (m *mockQuiverCatalog) Get(_ domain.Namespace) (*domain.Quiver, error) {
	return nil, nil
}

func (m *mockQuiverCatalog) List() ([]domain.Quiver, error) {
	return m.saved, nil
}

func TestOnQuiverAdded_CallsCatalogSave(t *testing.T) {
	catalog := &mockQuiverCatalog{}
	handler := OnQuiverAdded(catalog)

	quiver := domain.Quiver{
		Namespace: "github.com/org/repo",
		Manifest:  domain.QuiverManifest{Name: "Test"},
	}
	evt := asynxModels.Event[domain.Quiver]{
		EventName: "quiver.added",
		Aggregate: quiver,
	}

	handler(context.Background(), evt)

	require.Len(t, catalog.saved, 1)
	assert.Equal(t, quiver, catalog.saved[0])
}

func TestOnQuiverUpdated_CallsCatalogSave(t *testing.T) {
	catalog := &mockQuiverCatalog{}
	handler := OnQuiverUpdated(catalog)

	quiver := domain.Quiver{
		Namespace: "github.com/org/repo",
		Manifest:  domain.QuiverManifest{Name: "Updated"},
	}
	evt := asynxModels.Event[domain.Quiver]{
		EventName: "quiver.updated",
		Aggregate: quiver,
	}

	handler(context.Background(), evt)

	require.Len(t, catalog.saved, 1)
	assert.Equal(t, quiver, catalog.saved[0])
}

func TestOnQuiverRemoved_CallsCatalogDelete(t *testing.T) {
	catalog := &mockQuiverCatalog{}
	handler := OnQuiverRemoved(catalog)

	quiver := domain.Quiver{
		Namespace: "github.com/org/repo",
		Removed:   true,
	}
	evt := asynxModels.Event[domain.Quiver]{
		EventName: "quiver.removed",
		Aggregate: quiver,
	}

	handler(context.Background(), evt)

	require.Len(t, catalog.deleted, 1)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), catalog.deleted[0])
}
