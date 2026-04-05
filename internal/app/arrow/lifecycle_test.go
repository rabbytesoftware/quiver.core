package arrow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// --- Install tests ---

func TestRunInstall_NoDeps_InstallsRoot(t *testing.T) {
	ns := domain.Namespace("github.com/org/root")
	manifest := &domain.ArrowManifest{
		Name:    "Root",
		Version: "1.0.0",
	}

	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/home/root",
	}
	mm := &mocks.Manifold{}
	svc := testArrowService(t, mv, mm)
	mw := &mocks.Wizard{}
	svc.wizard = mw
	svc.deptree = mocks.DepTree([]domain.Namespace{ns}, nil)

	addArrowForTest(t, svc, ns, manifest)

	svc.runInstall(context.Background(), ns, nil)
	svc.asynxRuntime.WaitPublish()

	rt, err := svc.asynxRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, rt.State)
	assert.True(t, mw.WasExecuteCalled())
}

func TestRunInstall_DepInstallFails_RollsBack(t *testing.T) {
	ns := domain.Namespace("github.com/org/root")
	depNs := domain.Namespace("github.com/org/dep")

	depManifest := &domain.ArrowManifest{Name: "Dep", Version: "1.0.0"}
	rootManifest := &domain.ArrowManifest{
		Name:         "Root",
		Version:      "1.0.0",
		Dependencies: []domain.Namespace{depNs},
	}

	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: rootManifest},
		GetArrowPath:  "/home/root",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: depManifest}
	svc := testArrowService(t, mv, mm)

	callCount := 0
	mw := &mocks.Wizard{}
	mw.ExecuteErr = errors.New("dep install failed")
	svc.wizard = mw

	svc.deptree = func(ctx context.Context, root domain.Namespace, resolver deptree.ResolverFunc) ([]domain.Namespace, error) {
		callCount++
		return []domain.Namespace{depNs, root}, nil
	}

	addArrowForTest(t, svc, ns, rootManifest)
	addArrowForTest(t, svc, depNs, depManifest)

	svc.runInstall(context.Background(), ns, nil)

	rt, err := svc.asynxRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, rt.State)
}

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

	require.NoError(t, svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	}))
	svc.asynxRuntime.WaitPublish()

	err := svc.Install(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStateViolation)
}

func TestInstall_ValidArrow_LaunchesGoroutine(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/home/a",
	}
	mm := &mocks.Manifold{}
	svc := testArrowService(t, mv, mm)

	mw := &mocks.Wizard{}
	svc.wizard = mw
	svc.deptree = mocks.DepTree([]domain.Namespace{ns}, nil)

	addArrowForTest(t, svc, ns, manifest)

	err := svc.Install(context.Background(), ns, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		rt, getErr := svc.asynxRuntime.Get(context.Background(), ns.String())
		if getErr != nil {
			return false
		}
		return rt.State == domain.ArrowStateReady || rt.State == domain.ArrowStateAbsent
	}, 10*time.Second, 20*time.Millisecond)

	rt, err := svc.asynxRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, rt.State)
}

func TestRunInstall_DeptreeFails_EndsWithFailed(t *testing.T) {
	ns := domain.Namespace("github.com/org/root")
	manifest := &domain.ArrowManifest{Name: "Root", Version: "1.0.0"}

	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/home/root",
	}
	svc := testArrowService(t, mv, &mocks.Manifold{})
	svc.wizard = &mocks.Wizard{}
	svc.deptree = mocks.DepTree(nil, errors.New("cycle detected"))

	addArrowForTest(t, svc, ns, manifest)

	svc.runInstall(context.Background(), ns, nil)

	rt, err := svc.asynxRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, rt.State)

	require.NotNil(t, rt.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, rt.LastReturn.Outcome)
}

// --- Uninstall tests ---

func TestRunUninstall_NoOrphans_CleanupVault(t *testing.T) {
	ns := domain.Namespace("github.com/org/root")
	manifest := &domain.ArrowManifest{Name: "Root", Version: "1.0.0"}

	tv := &trackingVault{
		Vault: mocks.Vault{
			GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
			GetArrowPath:  "/home/root",
		},
	}
	mm := &mocks.Manifold{}
	svc := testArrowService(t, tv, mm)

	mw := &mocks.Wizard{}
	svc.wizard = mw
	svc.deptree = mocks.DepTree([]domain.Namespace{ns}, nil)

	addArrowForTest(t, svc, ns, manifest)

	require.NoError(t, svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	}))
	svc.asynxRuntime.WaitPublish()

	svc.runUninstall(context.Background(), ns, nil)

	rt, err := svc.asynxRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, rt.State)

	assert.Contains(t, tv.deletedNamespaces, ns)
}

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

	require.NoError(t, svc.catalog.Save(domain.Arrow{Namespace: rootNs, Manifest: *rootManifest}))

	require.NoError(t, svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateReady,
	}))
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

	require.NoError(t, svc.catalog.Save(domain.Arrow{Namespace: rootNs, Manifest: *rootManifest}))
	require.NoError(t, svc.catalog.Save(domain.Arrow{Namespace: depNs, Manifest: *depManifest}))

	require.NoError(t, svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateReady,
	}))
	svc.asynxRuntime.WaitPublish()
	require.NoError(t, svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    depNs,
		State: domain.ArrowStateReady,
	}))
	svc.asynxRuntime.WaitPublish()

	err := svc.Uninstall(context.Background(), depNs, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDependentsExist)
}

func TestUninstall_ValidReady_LaunchesGoroutine(t *testing.T) {
	ns := domain.Namespace("github.com/org/repo")
	manifest := &domain.ArrowManifest{Name: "A", Version: "1.0.0"}

	tv := &trackingVault{
		Vault: mocks.Vault{
			GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
			GetArrowPath:  "/home/a",
		},
	}
	svc := testArrowService(t, tv, &mocks.Manifold{})
	mw := &mocks.Wizard{}
	svc.wizard = mw
	svc.deptree = mocks.DepTree([]domain.Namespace{ns}, nil)

	addArrowForTest(t, svc, ns, manifest)

	require.NoError(t, svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	}))
	svc.asynxRuntime.WaitPublish()

	err := svc.Uninstall(context.Background(), ns, nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		rt, getErr := svc.asynxRuntime.Get(context.Background(), ns.String())
		if getErr != nil {
			return false
		}
		return rt.State == domain.ArrowStateAbsent
	}, 10*time.Second, 20*time.Millisecond)

	rt, err := svc.asynxRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, rt.State)
}
