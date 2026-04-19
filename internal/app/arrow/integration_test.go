package arrow

import (
	"context"
	"sync"
	"testing"

	"github.com/char2cs/asynx"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// integrationFixture wires a real ArrowService with mock engines.
type integrationFixture struct {
	svc      ArrowService
	inner    *arrowService
	axArrow  asynx.Asynx[domain.Arrow]
	vault    *mockIntegVault
	manifold *mockIntegManifold
	wizard   *mocks.Wizard
}

func newIntegrationFixture(t *testing.T) *integrationFixture {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)

	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	v := &mockIntegVault{
		manifests: map[string]*domain.ArrowManifest{},
		paths:     map[string]string{},
	}
	m := &mockIntegManifold{manifests: map[string]*domain.ArrowManifest{}}
	w := &mocks.Wizard{}

	store, err := arrowstore.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	// Build catalog with shared asynx instances
	cat, catErr := catalog.New(axArrow, axRuntime, store)
	require.NoError(t, catErr)

	svc, err := NewArrowBuilder().
		WithEngines(&engine.Container{
			Vault:     v,
			Manifold:  m,
			DepTree:   deptree.New(),
			Netbridge: &mocks.Netbridge{AllocatePort: 8080},
			Wizard:    w,
		}).
		WithAsynxArrow(axArrow).
		WithAsynxRuntime(axRuntime).
		WithCatalog(cat).
		WithOS(domain.OSLinuxAMD64).
		Build()
	require.NoError(t, err)

	inner := svc.(*arrowService)

	return &integrationFixture{
		svc:      svc,
		inner:    inner,
		axArrow:  axArrow,
		vault:    v,
		manifold: m,
		wizard:   w,
	}
}

// mockIntegVault stores manifests and paths in memory.
type mockIntegVault struct {
	mu        sync.Mutex
	manifests map[string]*domain.ArrowManifest
	paths     map[string]string
}

func (v *mockIntegVault) GetArrow(
	_ context.Context,
	ns domain.Namespace,
) (*vault.VaultEntry, string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	m, ok := v.manifests[ns.String()]
	if !ok {
		return nil, "", vault.ErrNotCached
	}

	return &vault.VaultEntry{Manifest: m}, v.paths[ns.String()], nil
}

func (v *mockIntegVault) PutArrow(
	_ context.Context,
	ns domain.Namespace,
	manifest *domain.ArrowManifest,
) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := ns.String()
	v.manifests[key] = manifest
	path := "/tmp/integ/" + key
	v.paths[key] = path
	return path, nil
}

func (v *mockIntegVault) DeleteArrow(_ context.Context, ns domain.Namespace) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := ns.String()
	delete(v.manifests, key)
	delete(v.paths, key)
	return nil
}

func (v *mockIntegVault) GetQuiver(
	_ context.Context,
	_ domain.Namespace,
) (*vault.QuiverVaultEntry, string, error) {
	return nil, "", vault.ErrNotCached
}

func (v *mockIntegVault) PutQuiver(
	_ context.Context,
	_ domain.Namespace,
	_ *domain.QuiverManifest,
) (string, error) {
	return "", nil
}

func (v *mockIntegVault) DeleteQuiver(_ context.Context, _ domain.Namespace) error {
	return nil
}

func (v *mockIntegVault) ListVersions(_ context.Context, _ domain.Namespace) ([]string, error) {
	return nil, nil
}

// mockIntegManifold returns manifests keyed by namespace string.
type mockIntegManifold struct {
	mu        sync.Mutex
	manifests map[string]*domain.ArrowManifest
}

func (m *mockIntegManifold) ResolveArrow(
	_ context.Context,
	ns domain.Namespace,
) (*domain.ArrowManifest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, ok := m.manifests[ns.String()]
	if !ok {
		return nil, apperrors.ErrFetchFailed
	}
	return manifest, nil
}

func (m *mockIntegManifold) ResolveQuiver(
	_ context.Context,
	_ domain.Namespace,
) (*domain.QuiverManifest, error) {
	return nil, apperrors.ErrFetchFailed
}

func (m *mockIntegManifold) ParseArrow(
	_ []byte,
) (*domain.ArrowManifest, error) {
	return nil, apperrors.ErrFetchFailed
}

func (m *mockIntegManifold) ResolveConstraint(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) (string, error) {
	return "", apperrors.ErrFetchFailed
}

func (m *mockIntegManifold) set(ns domain.Namespace, manifest *domain.ArrowManifest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manifests[ns.String()] = manifest
}

// --- Integration Tests ---

func TestIntegration_Add_ArrowAppearsInList(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow1")
	f.manifold.set(ns, &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "Arrow1", Version: "1.0.0"}})

	err := f.svc.Add(ctx, ns)
	require.NoError(t, err)

	f.axArrow.WaitPublish()

	list, err := f.svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, ns, list[0].Namespace)
}

func TestIntegration_Update_ManifestChanges(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow1")
	f.manifold.set(ns, &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "Arrow1", Version: "1.0.0"}})

	require.NoError(t, f.svc.Add(ctx, ns))
	f.axArrow.WaitPublish()

	f.manifold.set(ns, &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "Arrow1-Updated", Version: "2.0.0"}})
	require.NoError(t, f.vault.DeleteArrow(ctx, ns))

	_, updateErr := f.svc.Update(ctx, ns, UpdateOptions{})
	require.NoError(t, updateErr)
	f.axArrow.WaitPublish()

	detail, err := f.svc.GetDetail(ctx, ns)
	require.NoError(t, err)
	assert.Equal(t, "Arrow1-Updated", detail.Name)
}

func TestIntegration_Remove_DisappearsFromList(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow1")
	f.manifold.set(ns, &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "Arrow1", Version: "1.0.0"}})

	require.NoError(t, f.svc.Add(ctx, ns))
	f.axArrow.WaitPublish()

	require.NoError(t, f.svc.Remove(ctx, ns))
	f.axArrow.WaitPublish()

	list, err := f.svc.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestIntegration_BeginExecution_EmitsRunningState(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow1")
	// Include compiled Targets so resolveTarget can find OSLinuxAMD64.
	f.manifold.set(ns, &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "Arrow1", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Lifecycle: domain.TargetLifecycle{
					Execute: domainStep.StepList{domainStep.NewRunStep("run", "echo ok", false, "", false)},
				},
			},
		},
	})

	require.NoError(t, f.svc.Add(ctx, ns))
	f.axArrow.WaitPublish()

	// Seed runtime to Ready state so BeginExecution is allowed
	_, err := f.inner.asynxRuntime.Send(ctx, mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	f.inner.asynxRuntime.WaitPublish()

	err = f.svc.BeginExecution(ctx, ns, "_execute", nil)
	require.NoError(t, err)

	// BeginExecution emits a BeginExecution event that transitions state to Running,
	// then emits runtime.begun which the executor handles. Wait for both publishes.
	f.inner.asynxRuntime.WaitPublish() // BeginExecution → running
	f.inner.asynxRuntime.WaitPublish() // runtime.begun handler → EndExecution → ready

	detail, err := f.svc.GetDetail(ctx, ns)
	require.NoError(t, err)
	// Full pipeline completes: running → executor fires → ready
	assert.Equal(t, domain.ArrowStateReady, detail.State)
}

func TestIntegration_Stop_CancelsExecution(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow1")
	f.manifold.set(ns, &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "Arrow1", Version: "1.0.0"}})

	require.NoError(t, f.svc.Add(ctx, ns))
	f.axArrow.WaitPublish()

	// Seed runtime to Running state
	_, err := f.inner.asynxRuntime.Send(ctx, mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateRunning,
	})
	require.NoError(t, err)
	f.inner.asynxRuntime.WaitPublish()

	err = f.svc.Stop(ctx, ns)
	require.NoError(t, err)

	f.inner.asynxRuntime.WaitPublish()

	detail, err := f.svc.GetDetail(ctx, ns)
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateStopping, detail.State)
}

func TestIntegration_GetDetail_ReturnsManifestFromVault(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns1 := domain.Namespace("github.com/test/arrow1")

	f.manifold.set(ns1, &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "Arrow1", Version: "1.0.0"},
	})

	require.NoError(t, f.svc.Add(ctx, ns1))
	f.axArrow.WaitPublish()

	detail, err := f.svc.GetDetail(ctx, ns1)
	require.NoError(t, err)
	assert.Equal(t, "Arrow1", detail.Name)
}
