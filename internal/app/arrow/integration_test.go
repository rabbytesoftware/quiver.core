package arrow

import (
	"context"
	"sync"
	"testing"
	"time"

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
		manifests: map[string]*domain.Arrow{},
		paths:     map[string]string{},
	}
	m := &mockIntegManifold{manifests: map[string]*domain.Arrow{}}
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
	manifests map[string]*domain.Arrow
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

func (v *mockIntegVault) DeleteArrow(_ context.Context, ns domain.Namespace) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	key := ns.String()
	delete(v.manifests, key)
	delete(v.paths, key)
	return nil
}

func (v *mockIntegVault) WorkDir(_ context.Context, _ domain.Namespace) (string, error) {
	return "", nil
}

func (v *mockIntegVault) DeleteWorkDir(_ context.Context, _ domain.Namespace) error {
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

// mockIntegManifold returns manifests keyed by namespace string.
type mockIntegManifold struct {
	mu        sync.Mutex
	manifests map[string]*domain.Arrow
}

func (m *mockIntegManifold) ResolveArrow(
	_ context.Context,
	ns domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	manifest, ok := m.manifests[ns.String()]
	if !ok {
		return nil, nil, "", apperrors.ErrFetchFailed
	}
	return manifest, []byte("content"), "ARROW.md", nil
}

func (m *mockIntegManifold) ResolveQuiver(
	_ context.Context,
	_ domain.Namespace,
) (*domain.QuiverManifest, error) {
	return nil, apperrors.ErrFetchFailed
}

func (m *mockIntegManifold) ParseArrow(
	_ []byte,
) (*domain.Arrow, error) {
	return nil, apperrors.ErrFetchFailed
}

func (m *mockIntegManifold) ResolveConstraint(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) (string, error) {
	return "", apperrors.ErrFetchFailed
}

func (m *mockIntegManifold) set(ns domain.Namespace, manifest *domain.Arrow) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manifests[ns.String()] = manifest
}

// --- Integration Tests ---

func TestIntegration_Add_ArrowAppearsInList(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow1")
	f.manifold.set(ns, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Arrow1", Version: "1.0.0"}})

	err := f.svc.Add(ctx, ns)
	require.NoError(t, err)

	f.axArrow.WaitPublish()

	list, err := f.svc.List(ctx, nil)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, ns, list[0].Namespace)
}

func TestIntegration_Update_ManifestChanges(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow1")
	f.manifold.set(ns, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Arrow1", Version: "1.0.0"}})

	require.NoError(t, f.svc.Add(ctx, ns))
	f.axArrow.WaitPublish()

	f.manifold.set(ns, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Arrow1-Updated", Version: "2.0.0"}})
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
	f.manifold.set(ns, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Arrow1", Version: "1.0.0"}})

	require.NoError(t, f.svc.Add(ctx, ns))
	f.axArrow.WaitPublish()

	require.NoError(t, f.svc.Remove(ctx, ns))
	f.axArrow.WaitPublish()

	list, err := f.svc.List(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestIntegration_BeginExecution_EmitsRunningState(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow1")
	// Include compiled Targets so resolveTarget can find OSLinuxAMD64.
	f.manifold.set(ns, &domain.Arrow{
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
	f.manifold.set(ns, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Arrow1", Version: "1.0.0"}})

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

	assert.Eventually(t, func() bool {
		return f.wizard.WasCancelledFor(ns)
	}, 2*time.Second, 10*time.Millisecond)
}

func TestIntegration_GetDetail_ReturnsManifestFromVault(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns1 := domain.Namespace("github.com/test/arrow1")

	f.manifold.set(ns1, &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Name: "Arrow1", Version: "1.0.0"},
	})

	require.NoError(t, f.svc.Add(ctx, ns1))
	f.axArrow.WaitPublish()

	detail, err := f.svc.GetDetail(ctx, ns1)
	require.NoError(t, err)
	assert.Equal(t, "Arrow1", detail.Name)
}

func TestIntegration_GetManifest_BareNamespace_Success(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow1")
	f.manifold.set(ns, &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "TestArrow", Version: "1.0.0", Description: "A test arrow", Tags: []string{"test"}},
		Variables: []domain.Variable{{Name: "var1", Description: "First variable"}},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {},
		},
	})

	manifest, err := f.svc.GetManifest(ctx, ns)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	assert.Equal(t, ns, manifest.Namespace)
	assert.Equal(t, "TestArrow", manifest.Name)
	assert.Equal(t, "1.0.0", manifest.Version)
	assert.Equal(t, "A test arrow", manifest.Description)
	assert.Equal(t, []string{"test"}, manifest.Tags)
	assert.Len(t, manifest.Variables, 1)
	assert.NotNil(t, manifest.Manifest)
}

func TestIntegration_GetManifest_VersionedNamespace_ReturnsInvalidNamespace(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/arrow1@v1.0")
	f.manifold.set(ns, &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Name: "TestArrow", Version: "1.0.0"},
	})

	manifest, err := f.svc.GetManifest(ctx, ns)
	assert.Nil(t, manifest)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestIntegration_GetManifest_ResolveError_PropagatesError(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/nonexistent")
	// Don't set the manifest in manifold, so resolution fails

	manifest, err := f.svc.GetManifest(ctx, ns)
	assert.Nil(t, manifest)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

func TestIntegration_GetManifest_UninstalledArrow_ResolvesFromRegistry(t *testing.T) {
	// GetManifest should work for arrows not yet installed/added to catalog
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/external/uninstalled")
	f.manifold.set(ns, &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{
			Name:        "ExternalArrow",
			Version:     "2.5.1",
			Description: "An external uninstalled arrow",
			Tags:        []string{"external", "demo"},
		},
		Variables: []domain.Variable{
			{Name: "API_KEY", Description: "API key for service"},
			{Name: "TIMEOUT", Description: "Request timeout", Default: "30"},
		},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64:  {},
			domain.OSDarwinARM64: {},
		},
	})

	// Arrow is not added to service, only in manifold
	manifest, err := f.svc.GetManifest(ctx, ns)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	assert.Equal(t, "ExternalArrow", manifest.Name)
	assert.Equal(t, "2.5.1", manifest.Version)
	assert.Len(t, manifest.Variables, 2)
	assert.Len(t, manifest.Targets, 2)
}

func TestIntegration_GetManifest_InstalledArrow_ReturnsFreshManifest(t *testing.T) {
	// GetManifest resolves fresh from registry even for installed arrows
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns := domain.Namespace("github.com/test/installed")
	freshArrow := &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "InstalledArrow", Version: "1.0.0", Description: "Fresh version"},
		Variables: []domain.Variable{{Name: "fresh_var", Description: "Fresh variable"}},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {},
		},
	}
	f.manifold.set(ns, freshArrow)

	// Add to service/catalog (installs it)
	require.NoError(t, f.svc.Add(ctx, ns))
	f.axArrow.WaitPublish()

	// GetManifest should resolve from registry (fresh)
	manifest, err := f.svc.GetManifest(ctx, ns)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	assert.Equal(t, "InstalledArrow", manifest.Name)
	assert.Equal(t, "Fresh version", manifest.Description)
	assert.Len(t, manifest.Variables, 1)
}

func TestIntegration_GetManifest_MultipleVersions_ResolvesBareName(t *testing.T) {
	// GetManifest resolves bare namespace to a specific version in the registry
	f := newIntegrationFixture(t)
	ctx := context.Background()

	bare := domain.Namespace("github.com/test/versioned")

	// Set up v2.0.0 as the preferred version
	nsV2 := bare.WithRef("v2.0.0")
	v2Arrow := &domain.Arrow{
		Namespace: nsV2,
		ArrowMeta: domain.ArrowMeta{Name: "VersionedArrow", Version: "2.0.0", Description: "Version 2"},
		Variables: []domain.Variable{{Name: "v2_only", Description: "Only in v2"}},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64:  {},
			domain.OSDarwinARM64: {},
		},
	}
	f.manifold.set(nsV2, v2Arrow)

	// Also set bare namespace to point to v2
	// (in real registry, bare namespace would resolve to latest)
	v2BareCopy := *v2Arrow
	v2BareCopy.Namespace = bare
	f.manifold.set(bare, &v2BareCopy)

	// Add to catalog
	require.NoError(t, f.svc.Add(ctx, nsV2))
	f.axArrow.WaitPublish()

	// GetManifest on bare namespace
	manifest, err := f.svc.GetManifest(ctx, bare)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	assert.Equal(t, "VersionedArrow", manifest.Name)
	assert.Equal(t, "2.0.0", manifest.Version)
	assert.Len(t, manifest.Variables, 1)
	assert.Len(t, manifest.Targets, 2)
}
