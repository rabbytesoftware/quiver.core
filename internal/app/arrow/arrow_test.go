package arrow

import (
	"context"
	"errors"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestManifest(name string) *domain.ArrowManifest {
	return &domain.ArrowManifest{
		Name:        name,
		Version:     "1.0.0",
		Description: "A test arrow",
		Tags:        []string{"test"},
	}
}

func makeArrowServiceWithMocks(v vault.Vault, m *mocks.Manifold) *arrowService {
	return &arrowService{vault: v, manifold: m}
}

func testArrowService(t *testing.T, v vault.Vault, m *mocks.Manifold) *arrowService {
	t.Helper()
	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)
	catalog, err := arrowstore.NewArrowCatalogFromPath(":memory:")
	require.NoError(t, err)
	return &arrowService{
		asynxArrow:   axArrow,
		asynxRuntime: axRuntime,
		catalog:      catalog,
		vault:        v,
		manifold:     m,
	}
}

func addArrowForTest(t *testing.T, svc *arrowService, ns domain.Namespace, manifest *domain.ArrowManifest) {
	t.Helper()
	require.NoError(t, svc.catalog.Save(domain.Arrow{Namespace: ns, Manifest: *manifest}))
	require.NoError(t, svc.asynxArrow.Send(context.Background(), arrowcmds.AddArrow{
		Namespace: ns,
		Manifest:  *manifest,
	}))
	svc.asynxArrow.WaitPublish()
}

// --- Add ---

func TestAdd_InvalidNamespace_ReturnsErrInvalidNamespace(t *testing.T) {
	svc := testArrowService(t, &mocks.Vault{}, &mocks.Manifold{})

	err := svc.Add(context.Background(), "bad-namespace")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

func TestAdd_ManifoldFails_ReturnsErrFetchFailed(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	mm := &mocks.Manifold{ResolveArrowErr: errors.New("network error")}
	svc := testArrowService(t, mv, mm)

	err := svc.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchFailed)
}

func TestAdd_Success_ArrowSentToAsynx(t *testing.T) {
	manifest := makeTestManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc := testArrowService(t, mv, mm)

	err := svc.Add(context.Background(), "github.com/org/repo")
	require.NoError(t, err)

	svc.asynxArrow.WaitPublish()

	got, err := svc.asynxArrow.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), got.Namespace)
	assert.False(t, got.Removed)
}

func TestAdd_AlreadyExists_ReturnsError(t *testing.T) {
	manifest := makeTestManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc := testArrowService(t, mv, mm)

	require.NoError(t, svc.Add(context.Background(), "github.com/org/repo"))
	svc.asynxArrow.WaitPublish()

	err := svc.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
}

// --- Update ---

func TestUpdate_NotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	mm := &mocks.Manifold{}
	svc := testArrowService(t, mv, mm)

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUpdate_AlreadyRemoved_ReturnsErrAlreadyRemoved(t *testing.T) {
	manifest := makeTestManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc := testArrowService(t, mv, mm)

	addArrowForTest(t, svc, "github.com/org/repo", manifest)

	// Mark as removed via command
	require.NoError(t, svc.asynxArrow.Send(context.Background(), arrowcmds.RemoveArrow{Namespace: "github.com/org/repo"}))
	svc.asynxArrow.WaitPublish()

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyRemoved)
}

func TestUpdate_ManifoldFails_ReturnsErrFetchFailed(t *testing.T) {
	manifest := makeTestManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc := testArrowService(t, mv, mm)

	addArrowForTest(t, svc, "github.com/org/repo", manifest)

	// Now make manifold fail
	mm.ResolveArrowErr = errors.New("network error")
	mm.ResolveArrowManifest = nil

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchFailed)
}

func TestUpdate_Success_UpdatesArrow(t *testing.T) {
	manifest := makeTestManifest("MyArrow")
	updatedManifest := makeTestManifest("UpdatedArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc := testArrowService(t, mv, mm)

	addArrowForTest(t, svc, "github.com/org/repo", manifest)

	mm.ResolveArrowManifest = updatedManifest
	err := svc.Update(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	svc.asynxArrow.WaitPublish()

	got, err := svc.asynxArrow.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, "UpdatedArrow", got.Manifest.Name)
}

// --- Remove ---

func TestRemove_NotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	mm := &mocks.Manifold{}
	svc := testArrowService(t, mv, mm)

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRemove_AlreadyRemoved_ReturnsErrAlreadyRemoved(t *testing.T) {
	manifest := makeTestManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc := testArrowService(t, mv, mm)

	addArrowForTest(t, svc, "github.com/org/repo", manifest)
	require.NoError(t, svc.asynxArrow.Send(context.Background(), arrowcmds.RemoveArrow{Namespace: "github.com/org/repo"}))
	svc.asynxArrow.WaitPublish()

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyRemoved)
}

func TestRemove_Success_MarksArrowRemoved(t *testing.T) {
	manifest := makeTestManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc := testArrowService(t, mv, mm)

	addArrowForTest(t, svc, "github.com/org/repo", manifest)

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	svc.asynxArrow.WaitPublish()

	got, err := svc.asynxArrow.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.True(t, got.Removed)
}

func TestRemove_ActiveRuntime_ReturnsErrStateViolation(t *testing.T) {
	manifest := makeTestManifest("MyArrow")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc := testArrowService(t, mv, mm)

	addArrowForTest(t, svc, "github.com/org/repo", manifest)

	// Simulate a running runtime state
	require.NoError(t, svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    "github.com/org/repo",
		State: domain.ArrowStateRunning,
	}))
	svc.asynxRuntime.WaitPublish()

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStateViolation)
}

// --- List ---

func TestList_EmptyCatalog_ReturnsEmpty(t *testing.T) {
	svc := testArrowService(t, &mocks.Vault{}, &mocks.Manifold{})

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_FiltersRemovedArrows(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	svc := testArrowService(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, svc.catalog.Save(domain.Arrow{
		Namespace: "github.com/org/active",
		Manifest:  *manifest,
		Removed:   false,
	}))
	require.NoError(t, svc.catalog.Save(domain.Arrow{
		Namespace: "github.com/org/removed",
		Manifest:  *manifest,
		Removed:   true,
	}))

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, domain.Namespace("github.com/org/active"), result[0].Namespace)
}

func TestList_IncludesRuntimeState(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	svc := testArrowService(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, svc.catalog.Save(domain.Arrow{
		Namespace: ns,
		Manifest:  *manifest,
	}))

	err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, domain.ArrowStateReady, result[0].State)
}

func TestList_NoRuntime_ReturnsAbsentState(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	svc := testArrowService(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, svc.catalog.Save(domain.Arrow{
		Namespace: "github.com/org/repo",
		Manifest:  *manifest,
	}))

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, domain.ArrowStateAbsent, result[0].State)
}

// --- GetDetail ---

func TestGetDetail_NotFound_ReturnsError(t *testing.T) {
	svc := testArrowService(t, &mocks.Vault{}, &mocks.Manifold{})

	_, err := svc.GetDetail(context.Background(), "github.com/org/nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestGetDetail_NoRuntime_ReturnsAbsentState(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	svc := testArrowService(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, svc.catalog.Save(domain.Arrow{
		Namespace: ns,
		Manifest:  *manifest,
	}))

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, dto.State)
}

func TestGetDetail_WithRuntime_ReturnsCorrectState(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	svc := testArrowService(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, svc.catalog.Save(domain.Arrow{
		Namespace: ns,
		Manifest:  *manifest,
	}))

	err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, dto.State)
}

func TestGetDetail_WithIndirectDeps_IncludesDeps(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")
	indirectDeps := []domain.Namespace{"github.com/dep/one", "github.com/dep/two"}

	mv := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{
			Manifest:             manifest,
			IndirectDependencies: indirectDeps,
		},
	}
	svc := testArrowService(t, mv, &mocks.Manifold{})
	require.NoError(t, svc.catalog.Save(domain.Arrow{
		Namespace: ns,
		Manifest:  *manifest,
	}))

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, indirectDeps, dto.IndirectDependencies)
}

func TestGetDetail_VaultError_StillReturns(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	ns := domain.Namespace("github.com/org/repo")

	mv := &mocks.Vault{
		GetArrowErr: errors.New("vault unavailable"),
	}
	svc := testArrowService(t, mv, &mocks.Manifold{})
	require.NoError(t, svc.catalog.Save(domain.Arrow{
		Namespace: ns,
		Manifest:  *manifest,
	}))

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Nil(t, dto.IndirectDependencies)
	assert.Equal(t, ns, dto.Namespace)
}

func TestGetDetail_WithRuntimeExecution_PopulatesActiveRunAndLastReturn(t *testing.T) {
	manifest := makeTestManifest("Arrow")
	svc := testArrowService(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, svc.catalog.Save(domain.Arrow{
		Namespace: ns,
		Manifest:  *manifest,
	}))

	activeRun := &domainRuntime.RunRecord{
		Method:    "run",
		Variables: map[string]string{"key": "value"},
	}
	lastReturn := &domainRuntime.Return{
		Method:  "run",
		Outcome: domainRuntime.ExecutionOutcomeSuccess,
	}

	err := svc.asynxRuntime.Send(context.Background(), mocks.RuntimeCmdWithExecution{
		NS:         ns,
		State:      domain.ArrowStateReady,
		ActiveRun:  activeRun,
		LastReturn: lastReturn,
	})
	require.NoError(t, err)
	svc.asynxRuntime.WaitPublish()

	dto, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, dto.State)
	assert.Equal(t, activeRun, dto.ActiveRun)
	assert.Equal(t, lastReturn, dto.LastReturn)
}
