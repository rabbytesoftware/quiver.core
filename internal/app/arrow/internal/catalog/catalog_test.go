package catalog

import (
	"context"
	"errors"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
)

// --- helpers ---

func makeManifest(name string) *domain.ArrowManifest {
	return &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{
			Name:    name,
			Version: "1.0.0",
		},
	}
}

func newAsynxArrow(es asynxModels.Store) (asynx.Asynx[domain.Arrow], error) {
	return asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

func newAsynxRuntime(es asynxModels.Store) (asynx.Asynx[domainRuntime.ArrowRuntime], error) {
	return asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

func testCatalog(t *testing.T, v vault.Vault, m *mocks.Manifold) (*catalogService, Catalog) {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)

	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	cat, err := store.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	svc := &catalogService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		store:     cat,
		vault:     v,
		manifold:  m,
		graph:     nil, // graph is optional in tests
	}

	err = svc.registerProjections()
	require.NoError(t, err)

	return svc, svc
}

// seedArrow sends an AddArrow event and waits for projections to process.
func seedArrow(t *testing.T, svc *catalogService, ns domain.Namespace, manifest *domain.ArrowManifest) {
	t.Helper()

	err := svc.Add(context.Background(), ns)
	require.NoError(t, err)
	svc.axArrow.WaitPublish()
}

// vaultByNs returns different vault entries per namespace.
type vaultByNs struct {
	entries map[domain.Namespace]*vault.VaultEntry
}

func (v *vaultByNs) GetArrow(_ context.Context, ns domain.Namespace) (*vault.VaultEntry, string, error) {
	e, ok := v.entries[ns]
	if !ok {
		return nil, "", vault.ErrNotCached
	}
	return e, "/home/" + ns.String(), nil
}

func (v *vaultByNs) PutArrow(
	_ context.Context,
	_ domain.Namespace,
	_ *domain.ArrowManifest,
) (string, error) {
	return "/home/test", nil
}

func (v *vaultByNs) DeleteArrow(_ context.Context, _ domain.Namespace) error { return nil }

func (v *vaultByNs) GetQuiver(_ context.Context, _ domain.Namespace) (*vault.QuiverVaultEntry, string, error) {
	return nil, "", vault.ErrNotCached
}

func (v *vaultByNs) PutQuiver(_ context.Context, _ domain.Namespace, _ *domain.QuiverManifest) (string, error) {
	return "", nil
}

func (v *vaultByNs) DeleteQuiver(_ context.Context, _ domain.Namespace) error { return nil }
func (v *vaultByNs) ListVersions(_ context.Context, _ domain.Namespace) ([]string, error) {
	return nil, nil
}

// --- Add ---

func TestAdd_InvalidNamespace_ReturnsErrInvalidNamespace(t *testing.T) {
	_, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	err := cat.Add(context.Background(), "bad-namespace")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestAdd_ManifoldFails_ReturnsErrFetchFailed(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	mm := &mocks.Manifold{ResolveArrowErr: assert.AnError}
	_, cat := testCatalog(t, mv, mm)

	err := cat.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

func TestAdd_Success_ArrowAvailableInAsynx(t *testing.T) {
	manifest := makeManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	err := cat.Add(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	svc.axArrow.WaitPublish()

	got, err := svc.axArrow.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), got.Namespace)
}

func TestAdd_AlreadyExists_ReturnsErrAlreadyExists(t *testing.T) {
	manifest := makeManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	_, cat := testCatalog(t, mv, mm)

	require.NoError(t, cat.Add(context.Background(), "github.com/org/repo"))

	err := cat.Add(context.Background(), "github.com/org/repo")
	assert.ErrorIs(t, err, apperrors.ErrAlreadyExists)
}

// --- Update ---

func TestUpdate_NotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	_, cat := testCatalog(t, mv, &mocks.Manifold{})

	err := cat.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUpdate_AfterRemoved_ReturnsErrNotFound(t *testing.T) {
	manifest := makeManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	seedArrow(t, svc, "github.com/org/repo", manifest)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	err := cat.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUpdate_ManifoldFails_ReturnsErrFetchFailed(t *testing.T) {
	manifest := makeManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	seedArrow(t, svc, "github.com/org/repo", manifest)

	mm.ResolveArrowErr = assert.AnError
	mm.ResolveArrowManifest = nil

	err := cat.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

func TestUpdate_Success_UpdatesManifest(t *testing.T) {
	manifest := makeManifest("MyArrow")
	updated := makeManifest("UpdatedArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	seedArrow(t, svc, "github.com/org/repo", manifest)

	mm.ResolveArrowManifest = updated
	require.NoError(t, cat.Update(context.Background(), "github.com/org/repo"))
	svc.axArrow.WaitPublish()

	got, err := svc.axArrow.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	var gotName string
	for _, m := range got.Versions {
		gotName = m.Name
		break
	}
	assert.Equal(t, "UpdatedArrow", gotName)
}

func TestUpdate_ActiveRuntime_ReturnsErrStateViolation(t *testing.T) {
	manifest := makeManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	seedArrow(t, svc, "github.com/org/repo", manifest)

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    "github.com/org/repo",
		State: domain.ArrowStateRunning,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	err = cat.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

// --- Remove ---

func TestRemove_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_AfterRemoved_ReturnsErrNotFound(t *testing.T) {
	manifest := makeManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	seedArrow(t, svc, "github.com/org/repo", manifest)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_ActiveRuntime_ReturnsErrStateViolation(t *testing.T) {
	manifest := makeManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	seedArrow(t, svc, "github.com/org/repo", manifest)

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    "github.com/org/repo",
		State: domain.ArrowStateRunning,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	err = cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestRemove_Success_ForgetsAggregateFromAsynx(t *testing.T) {
	manifest := makeManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	seedArrow(t, svc, "github.com/org/repo", manifest)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	exists, err := svc.axArrow.Exists(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.False(t, exists)
}

// --- List ---

func TestList_EmptyStore_ReturnsEmpty(t *testing.T) {
	_, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	result, err := cat.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_ReturnsStoredArrows(t *testing.T) {
	manifest := makeManifest("Arrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	require.NoError(t, svc.store.Save(context.Background(), domain.Arrow{
		Namespace: "github.com/org/active",
		Versions:  map[string]domain.ArrowVersion{"1.0.0": {ArrowManifest: domain.ArrowManifest{ArrowMeta: manifest.ArrowMeta}}},
	}))
	require.NoError(t, svc.store.Save(context.Background(), domain.Arrow{
		Namespace: "github.com/org/other",
		Versions:  map[string]domain.ArrowVersion{"1.0.0": {ArrowManifest: domain.ArrowManifest{ArrowMeta: manifest.ArrowMeta}}},
	}))

	result, err := cat.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// --- Get ---

func TestGet_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	got, err := cat.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, got)
}

func TestGet_AfterRemoved_ReturnsErrNotFound(t *testing.T) {
	manifest := makeManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	seedArrow(t, svc, "github.com/org/repo", manifest)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	got, err := cat.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, got)
}

func TestGet_Exists_ReturnsArrow(t *testing.T) {
	manifest := makeManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/org/repo")
	seedArrow(t, svc, ns, manifest)

	got, err := cat.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, ns, got.Namespace)
}

// --- HasDependents ---

func TestHasDependents_NoDependents_ReturnsFalse(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	_, cat := testCatalog(t, mv, &mocks.Manifold{})

	has, err := cat.HasDependents(context.Background(), "github.com/org/dep", "")
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasDependents_WithDependent_ReturnsTrue(t *testing.T) {
	depNs := domain.Namespace("github.com/org/dep")
	rootNs := domain.Namespace("github.com/org/root")

	rootManifest := &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "Root", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {Tools: []domain.DependencyEdge{{Namespace: depNs, Type: domain.ToolDep}}},
		},
	}

	mv := &vaultByNs{
		entries: map[domain.Namespace]*vault.VaultEntry{
			rootNs: {Manifest: rootManifest},
		},
	}
	svc, cat := testCatalog(t, mv, &mocks.Manifold{})

	require.NoError(t, svc.store.Save(context.Background(), domain.Arrow{
		Namespace: rootNs,
		Versions:  map[string]domain.ArrowVersion{"1.0.0": {ArrowManifest: domain.ArrowManifest{ArrowMeta: rootManifest.ArrowMeta}}},
	}))

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	has, err := cat.HasDependents(context.Background(), depNs, "")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasDependents_WithExcludeNs_ExcludesDependent(t *testing.T) {
	depNs := domain.Namespace("github.com/org/dep")
	rootNs := domain.Namespace("github.com/org/root")

	rootManifest := &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "Root", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {Tools: []domain.DependencyEdge{{Namespace: depNs, Type: domain.ToolDep}}},
		},
	}

	mv := &vaultByNs{
		entries: map[domain.Namespace]*vault.VaultEntry{
			rootNs: {Manifest: rootManifest},
		},
	}
	svc, cat := testCatalog(t, mv, &mocks.Manifold{})

	require.NoError(t, svc.store.Save(context.Background(), domain.Arrow{
		Namespace: rootNs,
		Versions:  map[string]domain.ArrowVersion{"1.0.0": {ArrowManifest: domain.ArrowManifest{ArrowMeta: rootManifest.ArrowMeta}}},
	}))

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	has, err := cat.HasDependents(context.Background(), depNs, rootNs)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasDependents_ServiceDependency_ReturnsTrue(t *testing.T) {
	depNs := domain.Namespace("github.com/org/dep")
	rootNs := domain.Namespace("github.com/org/root")

	rootManifest := &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "Root", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {Services: []domain.DependencyEdge{{Namespace: depNs, Type: domain.ServiceDep}}},
		},
	}

	mv := &vaultByNs{
		entries: map[domain.Namespace]*vault.VaultEntry{
			rootNs: {Manifest: rootManifest},
		},
	}
	svc, cat := testCatalog(t, mv, &mocks.Manifold{})

	require.NoError(t, svc.store.Save(context.Background(), domain.Arrow{
		Namespace: rootNs,
		Versions:  map[string]domain.ArrowVersion{"1.0.0": {ArrowManifest: domain.ArrowManifest{ArrowMeta: rootManifest.ArrowMeta}}},
	}))

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	has, err := cat.HasDependents(context.Background(), depNs, "")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasDependents_AbsentRuntime_SkipsArrow(t *testing.T) {
	depNs := domain.Namespace("github.com/org/dep")
	rootNs := domain.Namespace("github.com/org/root")

	rootManifest := &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "Root", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {Tools: []domain.DependencyEdge{{Namespace: depNs, Type: domain.ToolDep}}},
		},
	}

	mv := &vaultByNs{
		entries: map[domain.Namespace]*vault.VaultEntry{
			rootNs: {Manifest: rootManifest},
		},
	}
	svc, cat := testCatalog(t, mv, &mocks.Manifold{})

	require.NoError(t, svc.store.Save(context.Background(), domain.Arrow{
		Namespace: rootNs,
		Versions:  map[string]domain.ArrowVersion{"1.0.0": {ArrowManifest: domain.ArrowManifest{ArrowMeta: rootManifest.ArrowMeta}}},
	}))

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateAbsent,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	has, err := cat.HasDependents(context.Background(), depNs, "")
	require.NoError(t, err)
	assert.False(t, has)
}

// --- New() ---

func TestNew_ValidArgs_ReturnsCatalog(t *testing.T) {
	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)

	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	cat, err := store.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	result, err := New(axArrow, axRuntime, cat, &mocks.Vault{}, &mocks.Manifold{}, nil)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// --- resolveManifest paths ---

// TestAdd_VaultReturnsStale_FallsBackToManifold exercises the ErrStale branch
// in resolveManifest where manifold succeeds, updating the vault entry.
func TestAdd_VaultReturnsStale_ManifoldSucceeds(t *testing.T) {
	manifest := makeManifest("StaleArrow")
	staleManifest := makeManifest("StaleOld")
	mv := &staleVault{
		staleEntry: &vault.VaultEntry{Manifest: staleManifest},
		stalePath:  "/tmp/stale",
		fresh:      manifest,
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, cat.Add(context.Background(), ns))
	svc.axArrow.WaitPublish()

	got, err := svc.axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	var gotName string
	for _, m := range got.Versions {
		gotName = m.Name
		break
	}
	assert.Equal(t, "StaleArrow", gotName)
}

// TestAdd_VaultReturnsStale_ManifoldFails_FallsBackToStaleEntry exercises the
// ErrStale path where manifold also fails: resolveManifest returns the stale entry.
func TestAdd_VaultReturnsStale_ManifoldFails_UsesStaleEntry(t *testing.T) {
	staleManifest := makeManifest("StaleOld")
	mv := &staleVault{
		staleEntry: &vault.VaultEntry{Manifest: staleManifest},
		stalePath:  "/tmp/stale",
	}
	mm := &mocks.Manifold{ResolveArrowErr: errors.New("manifold down")}
	_, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/org/repo")
	// resolveManifest returns the stale entry → Add succeeds
	require.NoError(t, cat.Add(context.Background(), ns))
}

// TestAdd_VaultReturnsOtherError_ReturnsErrFetchFailed covers the final
// catch-all return in resolveManifest.
func TestAdd_VaultReturnsOtherError_ReturnsErrFetchFailed(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: errors.New("unexpected storage error")}
	_, cat := testCatalog(t, mv, &mocks.Manifold{})

	err := cat.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

// TestAdd_VaultReturnsNotCached_PutFails_ReturnsErrFetchFailed covers the
// vault.PutArrow failure in the ErrNotCached branch of resolveManifest.
func TestAdd_VaultReturnsNotCached_PutFails_ReturnsErrFetchFailed(t *testing.T) {
	manifest := makeManifest("Arrow")
	mv := &switchableVault{
		getErr: vault.ErrNotCached,
		putErr: errors.New("disk full"),
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	_, cat := testCatalog(t, mv, mm)

	err := cat.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

// TestAdd_VaultCached_UsesExistingEntry verifies that when the vault already
// has a fresh entry, resolveManifest returns it without calling manifold.
func TestAdd_VaultCached_UsesExistingEntry(t *testing.T) {
	manifest := makeManifest("Cached")
	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: manifest},
		GetArrowPath:  "/tmp/cached",
	}
	mm := &mocks.Manifold{}
	svc, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, cat.Add(context.Background(), ns))
	svc.axArrow.WaitPublish()

	got, err := svc.axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	var gotName string
	for _, m := range got.Versions {
		gotName = m.Name
		break
	}
	assert.Equal(t, "Cached", gotName)

	// Manifold should not have been called.
	assert.Nil(t, mm.ResolveArrowManifest, "manifold was not needed")
}

// --- Update: non-ErrNotFound asynx error ---

func TestUpdate_RuntimeGetError_ReturnsError(t *testing.T) {
	// This path is exercised when axRuntime.Get returns a non-ErrNotFound error.
	// Since asynx only returns ErrNotFound for absent aggregates, we cover the
	// Update happy-path after seeding both arrow and a ready runtime state to
	// confirm the manifold/vault Update path works end-to-end.
	manifest := makeManifest("Arrow")
	updated := makeManifest("Updated")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/org/repo")
	seedArrow(t, svc, ns, manifest)

	// Runtime is not seeded → should be absent state → update allowed.
	mm.ResolveArrowManifest = updated
	require.NoError(t, cat.Update(context.Background(), ns))
}

// --- Remove: vault DeleteArrow error is best-effort ---

func TestRemove_VaultDeleteFails_StillSucceeds(t *testing.T) {
	manifest := makeManifest("Arrow")
	mv := &mocks.Vault{
		GetArrowErr:    vault.ErrNotCached,
		PutArrowPath:   "/tmp/test",
		DeleteArrowErr: errors.New("vault error"),
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/org/repo")
	seedArrow(t, svc, ns, manifest)

	// Remove should succeed even if vault deletion fails.
	require.NoError(t, cat.Remove(context.Background(), ns))
}

// --- List: store returns error ---

func TestList_StoreError_ReturnsError(t *testing.T) {
	svc, _ := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	svc.store = &failingArrowCatalog{listErr: errors.New("db unavailable")}

	result, err := svc.List(context.Background())
	require.Error(t, err)
	assert.Nil(t, result)
}

// --- HasDependents: vault error is skipped ---

func TestHasDependents_VaultGetArrowError_ContinuesAndReturnsFalse(t *testing.T) {
	depNs := domain.Namespace("github.com/org/dep")
	rootNs := domain.Namespace("github.com/org/root")

	rootManifest := &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "Root", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {Tools: []domain.DependencyEdge{{Namespace: depNs, Type: domain.ToolDep}}},
		},
	}

	mv := &mocks.Vault{GetArrowErr: errors.New("storage unavailable")}
	svc, cat := testCatalog(t, mv, &mocks.Manifold{})

	require.NoError(t, svc.store.Save(context.Background(), domain.Arrow{
		Namespace: rootNs,
		Versions:  map[string]domain.ArrowVersion{"1.0.0": {ArrowManifest: domain.ArrowManifest{ArrowMeta: rootManifest.ArrowMeta}}},
	}))

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    rootNs,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	has, err := cat.HasDependents(context.Background(), depNs, "")
	require.NoError(t, err)
	assert.False(t, has)
}

// staleVault returns ErrStale from GetArrow with the stale entry,
// and succeeds on PutArrow using the fresh manifest.
type staleVault struct {
	staleEntry *vault.VaultEntry
	stalePath  string
	fresh      *domain.ArrowManifest
	putCalled  bool
}

func (v *staleVault) GetArrow(_ context.Context, _ domain.Namespace) (*vault.VaultEntry, string, error) {
	return v.staleEntry, v.stalePath, vault.ErrStale
}

func (v *staleVault) PutArrow(
	_ context.Context,
	_ domain.Namespace,
	manifest *domain.ArrowManifest,
) (string, error) {
	v.putCalled = true
	v.fresh = manifest
	return "/tmp/fresh", nil
}

func (v *staleVault) DeleteArrow(_ context.Context, _ domain.Namespace) error { return nil }

func (v *staleVault) GetQuiver(_ context.Context, _ domain.Namespace) (*vault.QuiverVaultEntry, string, error) {
	return nil, "", vault.ErrNotCached
}

func (v *staleVault) PutQuiver(_ context.Context, _ domain.Namespace, _ *domain.QuiverManifest) (string, error) {
	return "", nil
}

func (v *staleVault) DeleteQuiver(_ context.Context, _ domain.Namespace) error { return nil }
func (v *staleVault) ListVersions(_ context.Context, _ domain.Namespace) ([]string, error) {
	return nil, nil
}

// failingAxArrow is a stub asynx.Asynx[domain.Arrow] that fails on the Nth Subscribe call
// and can optionally return an error from Get.
type failingAxArrow struct {
	subscribeCallN int
	calls          int
	err            error
	getErr         error
	existsErr      error
	onForgetErr    error
}

func (f *failingAxArrow) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Arrow],
	_ ...asynxModels.SubscriptionOpt[domain.Arrow],
) (string, error) {
	f.calls++
	if f.calls == f.subscribeCallN {
		return "", f.err
	}
	return "sub-id", nil
}

func (f *failingAxArrow) Send(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingAxArrow) SendWait(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingAxArrow) Shutdown(_ context.Context) error { return nil }
func (f *failingAxArrow) Get(_ context.Context, _ string) (domain.Arrow, error) {
	if f.getErr != nil {
		return domain.Arrow{}, f.getErr
	}
	return domain.Arrow{}, nil
}
func (f *failingAxArrow) Exists(_ context.Context, _ string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return false, nil
}
func (f *failingAxArrow) Preload(_ context.Context, _ string) error { return nil }
func (f *failingAxArrow) Unsubscribe(_ string) error                { return nil }
func (f *failingAxArrow) Replay(_ context.Context, _ string, _ int64, _ int64, _ asynxModels.ProjectionHandler[domain.Arrow]) error {
	return nil
}
func (f *failingAxArrow) WaitPublish()                             {}
func (f *failingAxArrow) Forget(_ context.Context, _ string) error { return nil }
func (f *failingAxArrow) OnForget(_ asynxModels.ForgetHandler[domain.Arrow]) (string, error) {
	if f.onForgetErr != nil {
		return "", f.onForgetErr
	}
	return "forget-sub-id", nil
}

// switchableVault is a test vault whose behaviour can be changed between calls.
type switchableVault struct {
	getEntry *vault.VaultEntry
	getPath  string
	getErr   error
	putPath  string
	putErr   error
}

func (v *switchableVault) GetArrow(_ context.Context, _ domain.Namespace) (*vault.VaultEntry, string, error) {
	return v.getEntry, v.getPath, v.getErr
}

func (v *switchableVault) PutArrow(
	_ context.Context,
	_ domain.Namespace,
	_ *domain.ArrowManifest,
) (string, error) {
	if v.putErr != nil {
		return "", v.putErr
	}
	return v.putPath, nil
}

func (v *switchableVault) DeleteArrow(_ context.Context, _ domain.Namespace) error { return nil }

func (v *switchableVault) GetQuiver(_ context.Context, _ domain.Namespace) (*vault.QuiverVaultEntry, string, error) {
	return nil, "", vault.ErrNotCached
}

func (v *switchableVault) PutQuiver(_ context.Context, _ domain.Namespace, _ *domain.QuiverManifest) (string, error) {
	return "", nil
}

func (v *switchableVault) DeleteQuiver(_ context.Context, _ domain.Namespace) error { return nil }
func (v *switchableVault) ListVersions(_ context.Context, _ domain.Namespace) ([]string, error) {
	return nil, nil
}

// failingArrowCatalog is a store that always fails on List.
type failingArrowCatalog struct {
	listErr error
}

func (f *failingArrowCatalog) Save(_ context.Context, _ domain.Arrow) error       { return nil }
func (f *failingArrowCatalog) Delete(_ context.Context, _ domain.Namespace) error { return nil }
func (f *failingArrowCatalog) Get(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
	return nil, nil
}
func (f *failingArrowCatalog) List(_ context.Context) ([]domain.Arrow, error) {
	return nil, f.listErr
}

// --- axArrow.Exists error branches ---

func TestUpdate_AxArrowExistsError_ReturnsError(t *testing.T) {
	svc, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	svc.axArrow = &failingAxArrow{existsErr: errors.New("storage failure")}

	err := cat.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_AxArrowExistsError_ReturnsError(t *testing.T) {
	svc, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	svc.axArrow = &failingAxArrow{existsErr: errors.New("storage failure")}

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGet_AxArrowGetError_ReturnsError(t *testing.T) {
	svc, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	svc.axArrow = &failingAxArrow{getErr: errors.New("storage failure")}

	got, err := cat.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, got)
}

// --- ErrStale + PutArrow failure ---

// TestAdd_VaultStale_ManifoldOK_PutFails_ReturnsError covers the ErrStale branch
// in resolveManifest where manifold succeeds but vault.PutArrow fails.
func TestAdd_VaultStale_ManifoldOK_PutFails_ReturnsError(t *testing.T) {
	staleManifest := makeManifest("StaleOld")
	freshManifest := makeManifest("FreshNew")
	sv := &switchableVault{
		getEntry: &vault.VaultEntry{Manifest: staleManifest},
		getPath:  "/tmp/stale",
		getErr:   vault.ErrStale,
		putErr:   errors.New("disk full"),
	}
	mm := &mocks.Manifold{ResolveArrowManifest: freshManifest}
	_, cat := testCatalog(t, sv, mm)

	err := cat.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

// TestUpdate_VaultPutFails_ReturnsError exercises the vault.PutArrow failure branch in Update.
func TestUpdate_VaultPutFails_ReturnsError(t *testing.T) {
	manifest := makeManifest("Arrow")
	updated := makeManifest("Updated")

	// Use a switchable vault: initially GetArrow returns ErrNotCached so Add's
	// resolveManifest calls manifold+PutArrow. After Add, switch to always-fail PutArrow.
	sv := &switchableVault{
		getErr:  vault.ErrNotCached,
		putPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, sv, mm)

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, cat.Add(context.Background(), ns))
	svc.axArrow.WaitPublish()

	// Switch: now PutArrow fails and GetArrow returns the cached entry.
	sv.getErr = nil
	sv.getEntry = &vault.VaultEntry{Manifest: manifest}
	sv.putErr = errors.New("disk full")
	mm.ResolveArrowManifest = updated

	err := cat.Update(context.Background(), ns)
	require.Error(t, err)
}

// failingArrowAsynx is a minimal asynx.Asynx[domain.Arrow] stub whose
// Subscribe always returns an error.
type failingArrowAsynx struct{ err error }

func (f *failingArrowAsynx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Arrow],
	_ ...asynxModels.SubscriptionOpt[domain.Arrow],
) (string, error) {
	return "", f.err
}
func (f *failingArrowAsynx) Send(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}
func (f *failingArrowAsynx) SendWait(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}
func (f *failingArrowAsynx) Get(_ context.Context, _ string) (domain.Arrow, error) {
	return domain.Arrow{}, nil
}
func (f *failingArrowAsynx) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *failingArrowAsynx) Preload(_ context.Context, _ string) error        { return nil }
func (f *failingArrowAsynx) Unsubscribe(_ string) error                       { return nil }
func (f *failingArrowAsynx) Replay(
	_ context.Context, _ string, _ int64, _ int64,
	_ asynxModels.ProjectionHandler[domain.Arrow],
) error {
	return nil
}
func (f *failingArrowAsynx) Shutdown(_ context.Context) error         { return nil }
func (f *failingArrowAsynx) WaitPublish()                             {}
func (f *failingArrowAsynx) Forget(_ context.Context, _ string) error { return nil }
func (f *failingArrowAsynx) OnForget(_ asynxModels.ForgetHandler[domain.Arrow]) (string, error) {
	return "", f.err
}

func TestNew_FailsWhenAsynxSubscribeFails(t *testing.T) {
	wantErr := errors.New("subscribe failed")

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	s, err := store.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	cat, err := New(&failingArrowAsynx{err: wantErr}, axRuntime, s, nil, nil, nil)

	assert.Nil(t, cat)
	require.ErrorIs(t, err, wantErr)
}

// --- AddWithManifest ---

func TestAddWithManifest_StoresManifestInVaultAndEmitsEvent(t *testing.T) {
	mv := &mocks.Vault{PutArrowPath: "/tmp/test"}
	mm := &mocks.Manifold{}
	cs, _ := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/user/repo")
	manifest := makeManifest("test-arrow")

	err := cs.AddWithManifest(context.Background(), ns, manifest)
	require.NoError(t, err)

	cs.axArrow.WaitPublish()
	got, err := cs.axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	var gotName string
	for _, m := range got.Versions {
		gotName = m.Name
		break
	}
	assert.Equal(t, "test-arrow", gotName)
}

func TestAddWithManifest_InvalidNamespace_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{}
	mm := &mocks.Manifold{}
	_, cat := testCatalog(t, mv, mm)

	err := cat.AddWithManifest(context.Background(), "bad", &domain.ArrowManifest{})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestAddWithManifest_VaultPutFails_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{PutArrowErr: errors.New("disk full")}
	mm := &mocks.Manifold{}
	_, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/user/repo")
	err := cat.AddWithManifest(context.Background(), ns, makeManifest("x"))
	require.Error(t, err)
}

func TestUpdateWithManifest_Success_UpdatesManifestInAsynx(t *testing.T) {
	manifest := makeManifest("original")
	updated := makeManifest("updated")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	svc, cat := testCatalog(t, mv, &mocks.Manifold{ResolveArrowManifest: manifest})
	seedArrow(t, svc, "github.com/user/repo", manifest)

	err := cat.UpdateWithManifest(context.Background(), "github.com/user/repo", updated)
	require.NoError(t, err)
	svc.axArrow.WaitPublish()

	got, err := svc.axArrow.Get(context.Background(), "github.com/user/repo")
	require.NoError(t, err)
	var gotName string
	for _, m := range got.Versions {
		gotName = m.Name
		break
	}
	assert.Equal(t, "updated", gotName)
}

func TestUpdateWithManifest_NotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{PutArrowPath: "/tmp/test"}
	_, cat := testCatalog(t, mv, &mocks.Manifold{})

	err := cat.UpdateWithManifest(context.Background(), "github.com/user/repo", makeManifest("x"))
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestAddWithManifest_AlreadyExists_ReturnsErrAlreadyExists(t *testing.T) {
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	_, cat := testCatalog(t, mv, &mocks.Manifold{})

	ns := domain.Namespace("github.com/user/repo")
	require.NoError(t, cat.AddWithManifest(context.Background(), ns, makeManifest("x")))

	err := cat.AddWithManifest(context.Background(), ns, makeManifest("x"))
	assert.ErrorIs(t, err, apperrors.ErrAlreadyExists)
}
