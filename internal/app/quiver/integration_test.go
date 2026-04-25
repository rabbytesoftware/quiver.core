package quiver

import (
	"context"
	"sync"
	"testing"

	"github.com/char2cs/asynx"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// integrationFixture wires a real QuiverService with mock engines.
type integrationFixture struct {
	svc      QuiverService
	axQuiver asynx.Asynx[domain.Quiver]
	vault    *mockIntegVault
	manifold *mockIntegManifold
}

func newIntegrationFixture(t *testing.T) *integrationFixture {
	t.Helper()

	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	store, err := quiverstore.NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	v := &mockIntegVault{
		manifests: map[string]*domain.QuiverManifest{},
		paths:     map[string]string{},
	}
	m := &mockIntegManifold{manifests: map[string]*domain.QuiverManifest{}}

	cat, catErr := catalog.New(axQuiver, store, v, m)
	require.NoError(t, catErr)

	svc, err := NewQuiverBuilder().
		WithEngines(&engine.Container{
			Vault:    v,
			Manifold: m,
		}).
		WithAsynxQuiver(axQuiver).
		WithCatalog(cat).
		Build()
	require.NoError(t, err)

	return &integrationFixture{
		svc:      svc,
		axQuiver: axQuiver,
		vault:    v,
		manifold: m,
	}
}

// mockIntegVault stores quiver manifests in memory.
type mockIntegVault struct {
	mu        sync.Mutex
	manifests map[string]*domain.QuiverManifest
	paths     map[string]string
}

func (v *mockIntegVault) GetArrow(
	_ context.Context,
	_ domain.Namespace,
) (vault.ManifestFile, error) {
	return vault.ManifestFile{}, vault.ErrNotCached
}

func (v *mockIntegVault) PutArrow(
	_ context.Context,
	_ domain.Namespace,
	_ vault.ManifestFile,
) error {
	return nil
}

func (v *mockIntegVault) DeleteArrow(_ context.Context, _ domain.Namespace) error {
	return nil
}

func (v *mockIntegVault) WorkDir(_ context.Context, _ domain.Namespace) (string, error) {
	return "", nil
}

func (v *mockIntegVault) GetQuiver(
	_ context.Context,
	ns domain.Namespace,
) (*vault.QuiverVaultEntry, string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	m, ok := v.manifests[ns.String()]
	if !ok {
		return nil, "", vault.ErrNotCached
	}

	return &vault.QuiverVaultEntry{Manifest: m}, v.paths[ns.String()], nil
}

func (v *mockIntegVault) PutQuiver(
	_ context.Context,
	ns domain.Namespace,
	manifest *domain.QuiverManifest,
) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := ns.String()
	v.manifests[key] = manifest
	path := "/tmp/integ/" + key
	v.paths[key] = path
	return path, nil
}

func (v *mockIntegVault) DeleteQuiver(_ context.Context, ns domain.Namespace) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := ns.String()
	delete(v.manifests, key)
	delete(v.paths, key)
	return nil
}

func (v *mockIntegVault) RenameArrow(
	_ context.Context,
	_ domain.Namespace,
	_ domain.Namespace,
) error {
	return nil
}

func (v *mockIntegVault) ListVersions(_ context.Context, _ domain.Namespace) ([]string, error) {
	return nil, nil
}

// mockIntegManifold returns quiver manifests keyed by namespace string.
type mockIntegManifold struct {
	mu        sync.Mutex
	manifests map[string]*domain.QuiverManifest
}

func (m *mockIntegManifold) ResolveArrow(
	_ context.Context,
	_ domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	return nil, nil, "", ErrFetchFailed
}

func (m *mockIntegManifold) ResolveQuiver(
	_ context.Context,
	ns domain.Namespace,
) (*domain.QuiverManifest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, ok := m.manifests[ns.String()]
	if !ok {
		return nil, ErrFetchFailed
	}
	return manifest, nil
}

func (m *mockIntegManifold) set(ns domain.Namespace, manifest *domain.QuiverManifest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manifests[ns.String()] = manifest
}

func (m *mockIntegManifold) ParseArrow(
	_ []byte,
) (*domain.Arrow, error) {
	return nil, nil
}

func (m *mockIntegManifold) ResolveConstraint(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) (string, error) {
	return "", nil
}

// --- Integration Tests ---

func TestIntegration_Add_QuiverAppearsInList(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/quiver1")
	f.manifold.set(ns, &domain.QuiverManifest{Name: "Quiver1"})

	err := f.svc.Add(ctx, ns)
	require.NoError(t, err)

	f.axQuiver.WaitPublish()

	list, err := f.svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, ns, list[0].Namespace)
}

func TestIntegration_Update_ManifestChanges(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/quiver1")
	f.manifold.set(ns, &domain.QuiverManifest{Name: "Quiver1"})

	require.NoError(t, f.svc.Add(ctx, ns))
	f.axQuiver.WaitPublish()

	f.manifold.set(ns, &domain.QuiverManifest{Name: "Quiver1Updated"})

	require.NoError(t, f.svc.Update(ctx, ns))
	f.axQuiver.WaitPublish()

	detail, err := f.svc.Get(ctx, ns)
	require.NoError(t, err)
	assert.Equal(t, "Quiver1Updated", detail.Manifest.Name)
}

func TestIntegration_Remove_DisappearsFromList(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/quiver1")
	f.manifold.set(ns, &domain.QuiverManifest{Name: "Quiver1"})

	require.NoError(t, f.svc.Add(ctx, ns))
	f.axQuiver.WaitPublish()

	require.NoError(t, f.svc.Remove(ctx, ns))
	f.axQuiver.WaitPublish()

	list, err := f.svc.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestIntegration_AddUpdateRemoveList_FullFlow(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns1 := domain.Namespace("github.com/test/quiver1")
	ns2 := domain.Namespace("github.com/test/quiver2")
	f.manifold.set(ns1, &domain.QuiverManifest{Name: "Quiver1", Tags: []string{"tag1"}})
	f.manifold.set(ns2, &domain.QuiverManifest{Name: "Quiver2", Tags: []string{"tag2"}})

	// Add both
	require.NoError(t, f.svc.Add(ctx, ns1))
	require.NoError(t, f.svc.Add(ctx, ns2))
	f.axQuiver.WaitPublish()

	list, err := f.svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)

	// Update ns1
	f.manifold.set(ns1, &domain.QuiverManifest{Name: "Quiver1V2", Tags: []string{"tag1", "updated"}})
	require.NoError(t, f.svc.Update(ctx, ns1))
	f.axQuiver.WaitPublish()

	detail, err := f.svc.Get(ctx, ns1)
	require.NoError(t, err)
	assert.Equal(t, "Quiver1V2", detail.Manifest.Name)

	// Remove ns2
	require.NoError(t, f.svc.Remove(ctx, ns2))
	f.axQuiver.WaitPublish()

	list, err = f.svc.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, ns1, list[0].Namespace)
}
