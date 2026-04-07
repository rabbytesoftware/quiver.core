package arrow

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callCountWizard fails only on the Nth call (failOnCall), succeeds for all others.
type callCountWizard struct {
	calls      atomic.Int64
	failOnCall int64
	failErr    error
}

func (w *callCountWizard) Execute(_ context.Context, _ wizard.RunRequest, _ wizard.StepReporter) error {
	n := w.calls.Add(1)
	if n == w.failOnCall {
		return w.failErr
	}
	return nil
}

func (w *callCountWizard) CallCount() int64 { return w.calls.Load() }

func (w *callCountWizard) Cancel(_ domain.Namespace) {}

func (w *callCountWizard) Shutdown(_ context.Context) error { return nil }

func (w *callCountWizard) RegisterDispatch(_ domainstep.StepType, _ wizard.DispatchFn) {}

// failingCatalog implements ArrowCatalog and returns an error on List.
type failingCatalog struct {
	listErr error
}

func (c *failingCatalog) Save(_ context.Context, _ domain.Arrow) error       { return nil }
func (c *failingCatalog) Delete(_ context.Context, _ domain.Namespace) error { return nil }
func (c *failingCatalog) Get(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
	return nil, nil
}
func (c *failingCatalog) List(_ context.Context) ([]domain.Arrow, error) {
	return nil, c.listErr
}

// countingVault returns GetArrowErr only on the Nth+ call to GetArrow.
type countingVault struct {
	mocks.Vault
	getArrowCalls int
	failAfter     int
	failErr       error
}

func (v *countingVault) GetArrow(_ context.Context, _ domain.Namespace) (*vault.VaultEntry, string, error) {
	v.getArrowCalls++
	if v.getArrowCalls > v.failAfter {
		return nil, "", v.failErr
	}
	return v.Vault.GetArrowEntry, v.Vault.GetArrowPath, nil
}

// ensure failingCatalog satisfies the interface at compile time
var _ arrowstore.ArrowCatalog = (*failingCatalog)(nil)

// trackingVault wraps mocks.Vault and records DeleteArrow calls.
type trackingVault struct {
	mocks.Vault
	deletedNamespaces []domain.Namespace
}

func (v *trackingVault) DeleteArrow(_ context.Context, ns domain.Namespace) error {
	v.deletedNamespaces = append(v.deletedNamespaces, ns)
	return nil
}

func (v *trackingVault) GetArrow(_ context.Context, _ domain.Namespace) (*vault.VaultEntry, string, error) {
	return v.GetArrowEntry, v.GetArrowPath, v.GetArrowErr
}

// vaultByNamespace is a test vault that returns different entries per namespace.
type vaultByNamespace struct {
	entries    map[domain.Namespace]*vault.VaultEntry
	deletedNSs []domain.Namespace
}

func (v *vaultByNamespace) GetArrow(_ context.Context, ns domain.Namespace) (*vault.VaultEntry, string, error) {
	entry, ok := v.entries[ns]
	if !ok {
		return nil, "", vault.ErrNotCached
	}
	return entry, "/home/" + ns.String(), nil
}

func (v *vaultByNamespace) PutArrow(_ context.Context, _ domain.Namespace, _ *domain.ArrowManifest, _ []domain.Namespace) (string, error) {
	return "/home/test", nil
}

func (v *vaultByNamespace) GetQuiver(_ context.Context, _ domain.Namespace) (*vault.QuiverVaultEntry, string, error) {
	return nil, "", vault.ErrNotCached
}

func (v *vaultByNamespace) PutQuiver(_ context.Context, _ domain.Namespace, _ *domain.QuiverManifest) (string, error) {
	return "", nil
}

func (v *vaultByNamespace) DeleteArrow(_ context.Context, ns domain.Namespace) error {
	v.deletedNSs = append(v.deletedNSs, ns)
	return nil
}

func (v *vaultByNamespace) DeleteQuiver(_ context.Context, _ domain.Namespace) error {
	return nil
}

// --- cleanupAfterUninstall ---

func TestCleanupAfterUninstall_NoVaultEntry_DeletesArrowAndReturns(t *testing.T) {
	tv := &trackingVault{
		Vault: mocks.Vault{GetArrowErr: errors.New("not cached")},
	}
	svc := testArrowService(t, tv, &mocks.Manifold{})
	ns := domain.Namespace("github.com/org/repo")

	svc.cleanupAfterUninstall(context.Background(), ns)

	require.Len(t, tv.deletedNamespaces, 1)
	assert.Equal(t, ns, tv.deletedNamespaces[0])
}

func TestCleanupAfterUninstall_WithVaultEntry_NoDeps_DeletesNs(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()
	ns := domain.Namespace("github.com/org/main")

	manifest := &domain.ArrowManifest{Name: "Main", Version: "1.0.0"}
	f.manifold.set(ns, manifest)
	require.NoError(t, f.svc.Add(ctx, ns))
	f.inner.asynxArrow.WaitPublish()

	// Put a vault entry for ns (no deps)
	_, err := f.vault.PutArrow(ctx, ns, manifest, nil)
	require.NoError(t, err)

	f.inner.cleanupAfterUninstall(ctx, ns)

	// vault entry should be deleted
	_, _, err = f.vault.GetArrow(ctx, ns)
	assert.Error(t, err, "expected vault entry to be deleted")
}

func TestCleanupAfterUninstall_WithOrphanDep_UninstallsAndDeletesDep(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns1 := domain.Namespace("github.com/org/main")
	ns2 := domain.Namespace("github.com/org/dep")

	depManifest := &domain.ArrowManifest{
		Name:    "Dep",
		Version: "1.0.0",
		Lifecycle: domain.Lifecycle{
			Uninstall: nil,
		},
	}
	mainManifest := &domain.ArrowManifest{
		Name:         "Main",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{ns2},
	}

	f.manifold.set(ns1, mainManifest)
	f.manifold.set(ns2, depManifest)

	require.NoError(t, f.svc.Add(ctx, ns1))
	f.inner.asynxArrow.WaitPublish()
	require.NoError(t, f.svc.Add(ctx, ns2))
	f.inner.asynxArrow.WaitPublish()

	// Put vault entries
	_, err := f.vault.PutArrow(ctx, ns1, mainManifest, nil)
	require.NoError(t, err)
	_, err = f.vault.PutArrow(ctx, ns2, depManifest, nil)
	require.NoError(t, err)

	// Seed ns2 runtime as Ready (orphan candidate: no other dependents)
	_, err = f.inner.asynxRuntime.Send(ctx, mocks.RuntimeCmd{
		NS:    ns2,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	f.inner.asynxRuntime.WaitPublish()

	f.inner.cleanupAfterUninstall(ctx, ns1)

	// ns1 vault entry should be deleted
	_, _, err = f.vault.GetArrow(ctx, ns1)
	assert.Error(t, err)
}

func TestCleanupAfterUninstall_DepHasOtherDependents_SkipsDep(t *testing.T) {
	f := newIntegrationFixture(t)
	ctx := context.Background()

	ns1 := domain.Namespace("github.com/org/main")
	ns2 := domain.Namespace("github.com/org/dep")
	ns3 := domain.Namespace("github.com/org/other")

	depManifest := &domain.ArrowManifest{Name: "Dep", Version: "1.0.0"}
	mainManifest := &domain.ArrowManifest{
		Name:         "Main",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{ns2},
	}
	otherManifest := &domain.ArrowManifest{
		Name:         "Other",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{ns2},
	}

	f.manifold.set(ns1, mainManifest)
	f.manifold.set(ns2, depManifest)
	f.manifold.set(ns3, otherManifest)

	for _, ns := range []domain.Namespace{ns1, ns2, ns3} {
		require.NoError(t, f.svc.Add(ctx, ns))
		f.inner.asynxArrow.WaitPublish()
	}

	// Put vault entries
	_, err := f.vault.PutArrow(ctx, ns1, mainManifest, nil)
	require.NoError(t, err)
	_, err = f.vault.PutArrow(ctx, ns2, depManifest, nil)
	require.NoError(t, err)
	_, err = f.vault.PutArrow(ctx, ns3, otherManifest, nil)
	require.NoError(t, err)

	// ns2 and ns3 are Ready
	for _, ns := range []domain.Namespace{ns2, ns3} {
		_, err = f.inner.asynxRuntime.Send(ctx, mocks.RuntimeCmd{
			NS:    ns,
			State: domain.ArrowStateReady,
		})
		require.NoError(t, err)
		f.inner.asynxRuntime.WaitPublish()
	}

	f.inner.cleanupAfterUninstall(ctx, ns1)

	// ns2 should NOT be deleted (ns3 still depends on it)
	_, _, err = f.vault.GetArrow(ctx, ns2)
	assert.NoError(t, err)

	// ns1 should be deleted
	_, _, err = f.vault.GetArrow(ctx, ns1)
	assert.Error(t, err)
}

// --- Install tests ---

func TestInstall_ArrowNotFound_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	err := svc.Install(context.Background(), "github.com/org/repo", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestInstall_AlreadyInstalled_ReturnsStateViolation(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/home/a",
	}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	addArrowForTest(t, svc, ns, manifest)

	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	err = svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStateViolation)
}

func TestInstall_ValidArrow_EmitsBeginExecution(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/home/a",
	}
	mm := &mocks.Manifold{}
	svc := testArrowService(t, mv, mm)

	addArrowForTest(t, svc, ns, manifest)

	err := svc.Install(context.Background(), ns, nil)
	require.NoError(t, err)

	svc.asynxRuntime.WaitPublish()

	rt, err := svc.asynxRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateInstalling, rt.State)
}

// --- Uninstall tests ---

func TestHasDependents_NoOtherArrows_ReturnsFalse(t *testing.T) {
	ns := domain.Namespace("github.com/org/dep")

	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	hasDeps, err := svc.hasDependents(context.Background(), ns, "")
	require.NoError(t, err)
	assert.False(t, hasDeps)
}

func TestHasDependents_OtherArrowDependsOnIt_ReturnsTrue(t *testing.T) {
	depNs := domain.Namespace("github.com/org/dep")
	rootNs := domain.Namespace("github.com/org/root")

	rootManifest := &domain.ArrowManifest{
		Name:         "Root",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{depNs},
	}

	mv := &vaultByNamespace{
		entries: map[domain.Namespace]*vault.VaultEntry{
			rootNs: {Manifest: rootManifest},
		},
	}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	require.NoError(t, svc.catalog.Save(context.Background(), domain.Arrow{Namespace: rootNs, Manifest: *rootManifest}))

	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	hasDeps, err := svc.hasDependents(context.Background(), depNs, "")
	require.NoError(t, err)
	assert.True(t, hasDeps)
}

func TestUninstall_NotReady_ReturnsStateViolation(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")

	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	err := svc.Uninstall(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStateViolation)
}

func TestUninstall_HasDependents_ReturnsError(t *testing.T) {
	depNs := domain.Namespace("github.com/org/dep")
	rootNs := domain.Namespace("github.com/org/root")

	rootManifest := &domain.ArrowManifest{
		Name:         "Root",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{depNs},
	}
	depManifest := &domain.ArrowManifest{Name: "Dep", Version: "1.0.0"}

	mv := &vaultByNamespace{
		entries: map[domain.Namespace]*vault.VaultEntry{
			rootNs: {Manifest: rootManifest},
			depNs:  {Manifest: depManifest},
		},
	}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	require.NoError(t, svc.catalog.Save(context.Background(), domain.Arrow{Namespace: rootNs, Manifest: *rootManifest}))
	require.NoError(t, svc.catalog.Save(context.Background(), domain.Arrow{Namespace: depNs, Manifest: *depManifest}))

	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()
	_, err = svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    depNs,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	err = svc.Uninstall(context.Background(), depNs, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDependentsExist)
}

func TestUninstall_ValidReady_EmitsBeginExecution(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	tv := &trackingVault{
		Vault: mocks.Vault{
			GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
			GetArrowPath:  "/home/a",
		},
	}
	svc := testArrowService(t, tv, &mocks.Manifold{})

	addArrowForTest(t, svc, ns, manifest)

	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	err = svc.Uninstall(context.Background(), ns, nil)
	require.NoError(t, err)

	svc.asynxRuntime.WaitPublish()

	rt, err := svc.asynxRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateUninstalling, rt.State)
}

// --- hasDependents additional paths ---

func TestHasDependents_CatalogListError_ReturnsError(t *testing.T) {
	ns := domain.Namespace("github.com/org/dep")

	mv := &mocks.Vault{}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	// Replace the real catalog with one that errors on List.
	svc.catalog = &failingCatalog{listErr: errors.New("db connection lost")}

	hasDeps, err := svc.hasDependents(context.Background(), ns, "")
	require.Error(t, err)
	assert.False(t, hasDeps)
}

func TestHasDependents_InstalledArrowHasIndirectDep_ReturnsTrue(t *testing.T) {
	depNs := domain.Namespace("github.com/org/dep")
	rootNs := domain.Namespace("github.com/org/root")

	rootManifest := &domain.ArrowManifest{
		Name:    "Root",
		Version: "1.0.0",
	}

	// Root has depNs only as an indirect dependency (not in manifest.Dependencies).
	mv := &vaultByNamespace{
		entries: map[domain.Namespace]*vault.VaultEntry{
			rootNs: {
				Manifest:             rootManifest,
				IndirectDependencies: []domain.Namespace{depNs},
			},
		},
	}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	require.NoError(t, svc.catalog.Save(context.Background(), domain.Arrow{
		Namespace: rootNs,
		Manifest:  *rootManifest,
	}))

	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	hasDeps, err := svc.hasDependents(context.Background(), depNs, "")
	require.NoError(t, err)
	assert.True(t, hasDeps)
}

// --- hasDependents additional paths ---

func TestHasDependents_AbsentRuntimeState_ReturnsFalse(t *testing.T) {
	depNs := domain.Namespace("github.com/org/dep")
	rootNs := domain.Namespace("github.com/org/root")

	rootManifest := &domain.ArrowManifest{
		Name:         "Root",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{depNs},
	}

	mv := &vaultByNamespace{
		entries: map[domain.Namespace]*vault.VaultEntry{
			rootNs: {Manifest: rootManifest},
		},
	}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	require.NoError(t, svc.catalog.Save(context.Background(), domain.Arrow{
		Namespace: rootNs,
		Manifest:  *rootManifest,
	}))

	// Root runtime is absent (state == absent) → hasDependents skips it.
	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateAbsent,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	hasDeps, err := svc.hasDependents(context.Background(), depNs, "")
	require.NoError(t, err)
	assert.False(t, hasDeps)
}

func TestHasDependents_VaultGetArrowError_Continues(t *testing.T) {
	depNs := domain.Namespace("github.com/org/dep")
	rootNs := domain.Namespace("github.com/org/root")

	rootManifest := &domain.ArrowManifest{
		Name:         "Root",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{depNs},
	}

	// Vault returns error for all GetArrow calls.
	mv := &mocks.Vault{GetArrowErr: errors.New("storage unavailable")}
	svc := testArrowService(t, mv, &mocks.Manifold{})

	require.NoError(t, svc.catalog.Save(context.Background(), domain.Arrow{
		Namespace: rootNs,
		Manifest:  *rootManifest,
	}))

	// rootNs is ready so it passes the runtime check.
	_, err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	// Vault.GetArrow error → continue → does not return true.
	hasDeps, err := svc.hasDependents(context.Background(), depNs, "")
	require.NoError(t, err)
	assert.False(t, hasDeps)
}
