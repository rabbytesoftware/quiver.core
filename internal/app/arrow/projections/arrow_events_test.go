package projections

import (
	"context"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockArrowCatalog struct {
	saved   []domain.Arrow
	deleted []domain.Namespace
}

func (m *mockArrowCatalog) Save(a domain.Arrow) error {
	m.saved = append(m.saved, a)
	return nil
}

func (m *mockArrowCatalog) Delete(ns domain.Namespace) error {
	m.deleted = append(m.deleted, ns)
	return nil
}

func (m *mockArrowCatalog) Get(_ domain.Namespace) (*domain.Arrow, error) {
	return nil, nil
}

func (m *mockArrowCatalog) List() ([]domain.Arrow, error) {
	return m.saved, nil
}

func TestOnArrowAdded_CallsCatalogSave(t *testing.T) {
	catalog := &mockArrowCatalog{}
	handler := OnArrowAdded(catalog)

	arrow := domain.Arrow{
		Namespace: "github.com/org/repo",
		Manifest:  domain.ArrowManifest{Name: "Test", Version: "1.0.0"},
	}
	evt := asynxModels.Event[domain.Arrow]{
		EventName: "arrow.added",
		Aggregate: arrow,
	}

	handler(context.Background(), evt)

	require.Len(t, catalog.saved, 1)
	assert.Equal(t, arrow, catalog.saved[0])
}

func TestOnArrowUpdated_CallsCatalogSave(t *testing.T) {
	catalog := &mockArrowCatalog{}
	handler := OnArrowUpdated(catalog)

	arrow := domain.Arrow{
		Namespace: "github.com/org/repo",
		Manifest:  domain.ArrowManifest{Name: "Updated", Version: "2.0.0"},
	}
	evt := asynxModels.Event[domain.Arrow]{
		EventName: "arrow.updated",
		Aggregate: arrow,
	}

	handler(context.Background(), evt)

	require.Len(t, catalog.saved, 1)
	assert.Equal(t, arrow, catalog.saved[0])
}

func TestOnArrowRemoved_CallsCatalogDelete(t *testing.T) {
	catalog := &mockArrowCatalog{}
	handler := OnArrowRemoved(catalog)

	arrow := domain.Arrow{
		Namespace: "github.com/org/repo",
		Removed:   true,
	}
	evt := asynxModels.Event[domain.Arrow]{
		EventName: "arrow.removed",
		Aggregate: arrow,
	}

	handler(context.Background(), evt)

	require.Len(t, catalog.deleted, 1)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), catalog.deleted[0])
}
