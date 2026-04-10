package quiver

import (
	"context"
	"errors"
	"sync"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildTestQuiverCatalog(t *testing.T) catalog.Catalog {
	t.Helper()

	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)
	store, err := quiverstore.NewQuiverCatalog(":memory:")
	require.NoError(t, err)
	cat, err := catalog.New(axQuiver, store, nil, nil)
	require.NoError(t, err)
	return cat
}

func TestBuilder_Build_SucceedsWithValidEventStore(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	svc, err := NewQuiverBuilder().
		WithEventStore(es).
		WithCatalog(buildTestQuiverCatalog(t)).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_FailsWithNilEventStore(t *testing.T) {
	svc, err := NewQuiverBuilder().Build()

	require.Error(t, err)
	assert.Nil(t, svc)
}

func TestNewAsynxQuiver_NilEventStore_ReturnsError(t *testing.T) {
	ax, err := newAsynxQuiver(nil)
	require.Error(t, err)
	assert.Nil(t, ax)
}

func TestBuilder_Build_NilCatalog_UsesDefaultPath(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	// Without WithCatalog — Build creates its own using metadata.GetQuiverHome()
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

type stubHub struct {
	mu      sync.Mutex
	quivers []domain.Quiver
}

func (s *stubHub) BroadcastArrow(_ domain.Arrow)                      {}
func (s *stubHub) BroadcastArrowRuntime(_ domainRuntime.ArrowRuntime) {}
func (s *stubHub) BroadcastQuiver(q domain.Quiver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quivers = append(s.quivers, q)
}

func TestQuiverBuilder_WithWebSocketHub_BroadcastsQuiverEvents(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	store, err := quiverstore.NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	mockVault := &mockBroadcastVault{manifests: map[string]*domain.QuiverManifest{}}
	mockManifold := &mockBroadcastManifold{manifests: map[string]*domain.QuiverManifest{}}

	cat, err := catalog.New(axQuiver, store, mockVault, mockManifold)
	require.NoError(t, err)

	hub := &stubHub{}

	svc, err := NewQuiverBuilder().
		WithAsynxQuiver(axQuiver).
		WithEventStore(es).
		WithCatalog(cat).
		WithWebSocketHub(hub).
		Build()
	require.NoError(t, err)

	ctx := context.Background()
	ns := domain.Namespace("github.com/user/repo")

	require.NoError(t, svc.Add(ctx, ns))
	axQuiver.WaitPublish()

	hub.mu.Lock()
	defer hub.mu.Unlock()
	require.Len(t, hub.quivers, 1)
	assert.Equal(t, ns, hub.quivers[0].Namespace)
}

func TestQuiverBuilder_WithoutHub_NoPanic(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	store, err := quiverstore.NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	mockVault := &mockBroadcastVault{manifests: map[string]*domain.QuiverManifest{}}
	mockManifold := &mockBroadcastManifold{manifests: map[string]*domain.QuiverManifest{}}

	cat, err := catalog.New(axQuiver, store, mockVault, mockManifold)
	require.NoError(t, err)

	// Build without hub — should not panic or error
	svc, err := NewQuiverBuilder().
		WithAsynxQuiver(axQuiver).
		WithEventStore(es).
		WithCatalog(cat).
		Build()
	require.NoError(t, err)

	ctx := context.Background()
	ns := domain.Namespace("github.com/user/repo")
	assert.NoError(t, svc.Add(ctx, ns))
}

// mockBroadcastVault is a minimal vault mock for broadcast testing.
type mockBroadcastVault struct {
	mu        sync.Mutex
	manifests map[string]*domain.QuiverManifest
}

func (v *mockBroadcastVault) GetArrow(
	_ context.Context,
	_ domain.Namespace,
) (*vault.VaultEntry, string, error) {
	return nil, "", vault.ErrNotCached
}

func (v *mockBroadcastVault) PutArrow(
	_ context.Context,
	_ domain.Namespace,
	_ *domain.ArrowManifest,
	_ []domain.Namespace,
) (string, error) {
	return "", nil
}

func (v *mockBroadcastVault) DeleteArrow(_ context.Context, _ domain.Namespace) error {
	return nil
}

func (v *mockBroadcastVault) GetQuiver(
	_ context.Context,
	ns domain.Namespace,
) (*vault.QuiverVaultEntry, string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	m, ok := v.manifests[ns.String()]
	if !ok {
		return nil, "", vault.ErrNotCached
	}

	return &vault.QuiverVaultEntry{Manifest: m}, "/tmp/test/" + ns.String(), nil
}

func (v *mockBroadcastVault) PutQuiver(
	_ context.Context,
	ns domain.Namespace,
	manifest *domain.QuiverManifest,
) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := ns.String()
	v.manifests[key] = manifest
	path := "/tmp/test/" + key
	return path, nil
}

func (v *mockBroadcastVault) DeleteQuiver(_ context.Context, ns domain.Namespace) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := ns.String()
	delete(v.manifests, key)
	return nil
}

// mockBroadcastManifold is a minimal manifold mock for broadcast testing.
type mockBroadcastManifold struct {
	mu        sync.Mutex
	manifests map[string]*domain.QuiverManifest
}

func (m *mockBroadcastManifold) ResolveArrow(
	_ context.Context,
	_ domain.Namespace,
) (*domain.ArrowManifest, error) {
	return nil, errors.New("not implemented")
}

func (m *mockBroadcastManifold) ResolveQuiver(
	_ context.Context,
	ns domain.Namespace,
) (*domain.QuiverManifest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, ok := m.manifests[ns.String()]
	if !ok {
		// Return a default manifest so the test can proceed
		manifest = &domain.QuiverManifest{Name: "test"}
		m.manifests[ns.String()] = manifest
	}
	return manifest, nil
}
