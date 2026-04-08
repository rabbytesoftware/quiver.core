package quiver

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCatalog is a test double for catalog.Catalog.
type mockCatalog struct {
	addErr     error
	updateErr  error
	removeErr  error
	listResult []domain.Quiver
	listErr    error
	getRes     *domain.Quiver
	getErr     error
}

func (m *mockCatalog) Add(_ context.Context, _ domain.Namespace) error {
	return m.addErr
}

func (m *mockCatalog) Update(_ context.Context, _ domain.Namespace) error {
	return m.updateErr
}

func (m *mockCatalog) Remove(_ context.Context, _ domain.Namespace) error {
	return m.removeErr
}

func (m *mockCatalog) List(_ context.Context) ([]domain.Quiver, error) {
	return m.listResult, m.listErr
}

func (m *mockCatalog) Get(_ context.Context, _ domain.Namespace) (*domain.Quiver, error) {
	return m.getRes, m.getErr
}

func newTestService(cat catalog.Catalog) *quiverService {
	return &quiverService{catalog: cat}
}

// --- Add ---

func TestAdd_DelegatesToCatalog_ReturnsNil(t *testing.T) {
	svc := newTestService(&mockCatalog{})

	err := svc.Add(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
}

func TestAdd_CatalogError_PropagatesError(t *testing.T) {
	svc := newTestService(&mockCatalog{addErr: ErrInvalidNamespace})

	err := svc.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

// --- Update ---

func TestUpdate_DelegatesToCatalog_ReturnsNil(t *testing.T) {
	svc := newTestService(&mockCatalog{})

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
}

func TestUpdate_CatalogError_PropagatesError(t *testing.T) {
	svc := newTestService(&mockCatalog{updateErr: ErrNotFound})

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

// --- Remove ---

func TestRemove_DelegatesToCatalog_ReturnsNil(t *testing.T) {
	svc := newTestService(&mockCatalog{})

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
}

func TestRemove_CatalogError_PropagatesError(t *testing.T) {
	svc := newTestService(&mockCatalog{removeErr: ErrAlreadyRemoved})

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyRemoved)
}

// --- List ---

func TestList_EmptyCatalog_ReturnsEmpty(t *testing.T) {
	svc := newTestService(&mockCatalog{listResult: []domain.Quiver{}})

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_MapsQuiversToDTO(t *testing.T) {
	quivers := []domain.Quiver{
		{
			Namespace: "github.com/org/repo",
			Manifest: domain.QuiverManifest{
				Name:        "TestQuiver",
				Description: "Desc",
				Tags:        []string{"a", "b"},
			},
			Removed: false,
		},
	}
	svc := newTestService(&mockCatalog{listResult: quivers})

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), result[0].Namespace)
	assert.Equal(t, "TestQuiver", result[0].Name)
	assert.Equal(t, "Desc", result[0].Description)
	assert.Equal(t, []string{"a", "b"}, result[0].Tags)
}

func TestList_CatalogError_ReturnsError(t *testing.T) {
	svc := newTestService(&mockCatalog{listErr: errors.New("store error")})

	_, err := svc.List(context.Background())
	require.Error(t, err)
}

// --- Get ---

func TestGet_NotFound_ReturnsError(t *testing.T) {
	svc := newTestService(&mockCatalog{getErr: ErrNotFound})

	detail, err := svc.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, detail)
}

func TestGet_Found_ReturnsMappedDTO(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	q := &domain.Quiver{
		Namespace: ns,
		Manifest:  domain.QuiverManifest{Name: "TestQuiver", Description: "Desc"},
		Removed:   false,
	}
	svc := newTestService(&mockCatalog{getRes: q})

	detail, err := svc.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, ns, detail.Namespace)
	assert.Equal(t, "TestQuiver", detail.Manifest.Name)
	assert.False(t, detail.Removed)
}

func TestGet_RemovedQuiver_ReturnsRemovedTrue(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	q := &domain.Quiver{
		Namespace: ns,
		Manifest:  domain.QuiverManifest{Name: "TestQuiver"},
		Removed:   true,
	}
	svc := newTestService(&mockCatalog{getRes: q})

	detail, err := svc.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.True(t, detail.Removed)
}
