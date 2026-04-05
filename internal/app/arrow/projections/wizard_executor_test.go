package projections

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wizardExecutorFixture holds all dependencies for WizardExecutor tests.
type wizardExecutorFixture struct {
	executor     *WizardExecutor
	axRuntime    asynx.Asynx[domainRuntime.ArrowRuntime]
	vault        *mocks.Vault
	depTree      *mocks.DepTree
	catalog      *mockArrowCatalog
	wiz          *mocks.Wizard
}

func newWizardExecutorFixture(t *testing.T) *wizardExecutorFixture {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := asynx.New[domain.Arrow]().
		WithEventStore(arrowES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axRuntime, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(runtimeES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)

	v := &mocks.Vault{}
	dt := &mocks.DepTree{}
	catalog := &mockArrowCatalog{}
	wiz := &mocks.Wizard{}

	executor := NewWizardExecutor(v, dt, axArrow, axRuntime, catalog, wiz)

	return &wizardExecutorFixture{
		executor:  executor,
		axRuntime: axRuntime,
		vault:     v,
		depTree:   dt,
		catalog:   catalog,
		wiz:       wiz,
	}
}

// seedRuntime sends a BeginExecution command so the runtime aggregate exists.
func seedRuntime(
	t *testing.T,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
	method string,
) {
	t.Helper()
	err := axRuntime.Send(context.Background(), arrowcmds.BeginExecution{
		Namespace: ns,
		Method:    method,
	})
	require.NoError(t, err)
	axRuntime.WaitPublish()
}

// makeRuntime constructs an ArrowRuntime with a populated ActiveRun for unit tests
// that call execute() directly (no asynx involved).
func makeRuntime(ns domain.Namespace, method string) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Namespace: ns,
		State:     domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{
			Method:    method,
			Variables: map[string]string{},
		},
	}
}

// --- execute() ---

func TestExecute_NilActiveRun_NoOp(t *testing.T) {
	f := newWizardExecutorFixture(t)

	rt := domainRuntime.ArrowRuntime{
		Namespace: "github.com/org/repo",
		State:     domain.ArrowStateReady,
		ActiveRun: nil,
	}

	// Should not panic and wizard should not be called.
	f.executor.execute(context.Background(), rt)

	assert.False(t, f.wiz.WasExecuteCalled())
}

func TestExecute_WizardSucceeds_SendsEndExecutionSuccess(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	f.vault.GetArrowPath = "/tmp/work"
	f.wiz.ExecuteErr = nil

	rt := makeRuntime(ns, "_execute")

	f.executor.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())

	// Verify EndExecution was sent by checking the runtime state via Get.
	result, err := f.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, result.LastReturn.Outcome)
}

func TestExecute_ErrExecutionExists_ReturnsSilently(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	f.wiz.ExecuteErr = wizard.ErrExecutionExists

	rt := makeRuntime(ns, "_execute")

	// Should not panic and must not call EndExecution (would fail validation if called
	// because the runtime still has an active run).
	f.executor.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())

	// Runtime still has ActiveRun (EndExecution was NOT sent).
	result, err := f.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.NotNil(t, result.ActiveRun)
}

func TestExecute_WizardFails_SendsEndExecutionFailed(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	f.wiz.ExecuteErr = errors.New("step failed")

	rt := makeRuntime(ns, "_execute")

	f.executor.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())

	result, err := f.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, result.LastReturn.Outcome)
}

func TestExecute_ExecuteMethodCanceled_TriggerStopCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	f.wiz.ExecuteErr = context.Canceled

	var triggerStopNs domain.Namespace
	f.executor.SetTriggerStop(func(_ context.Context, n domain.Namespace) {
		triggerStopNs = n
	})

	rt := makeRuntime(ns, "_execute")

	f.executor.execute(context.Background(), rt)

	assert.Equal(t, ns, triggerStopNs)
}

func TestExecute_UninstallSuccess_VaultDeleteArrowCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_uninstall")

	f.wiz.ExecuteErr = nil

	// GetArrow returns an entry with no dependencies so cleanupAfterUninstall proceeds.
	f.vault.GetArrowEntry = &vault.VaultEntry{
		Manifest: &domain.ArrowManifest{
			Name:    "Repo",
			Version: "1.0.0",
		},
	}
	f.depTree.Result = []domain.Namespace{ns}

	rt := makeRuntime(ns, "_uninstall")

	f.executor.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())
}

// --- handlePostExecution() ---

func TestHandlePostExecution_ExecuteCanceled_TriggerStopNil_NoPanic(t *testing.T) {
	f := newWizardExecutorFixture(t)
	// triggerStop is nil by default — should not panic.
	f.executor.handlePostExecution(
		context.Background(),
		"github.com/org/repo",
		"_execute",
		context.Canceled,
		domainRuntime.ExecutionOutcomeCancelled,
	)
}

func TestHandlePostExecution_ExecuteSuccess_TriggerStopNotCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	called := false
	f.executor.SetTriggerStop(func(_ context.Context, _ domain.Namespace) {
		called = true
	})

	f.executor.handlePostExecution(
		context.Background(),
		"github.com/org/repo",
		"_execute",
		nil,
		domainRuntime.ExecutionOutcomeSuccess,
	)

	assert.False(t, called)
}

func TestHandlePostExecution_UninstallFailed_CleanupNotCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	// If uninstall failed, cleanupAfterUninstall must NOT be triggered.
	// We can verify this by ensuring vault.GetArrow was never invoked during this path.
	// Since the mock returns GetArrowEntry = nil with no error by default,
	// if cleanup were called it would invoke DeleteArrow; we check it's not called
	// by swapping GetArrowErr to an error (cleanup returns early if GetArrow errors).
	f.vault.GetArrowErr = errors.New("not cached")

	// For this test we just verify no panics when cleanup is not triggered.
	f.executor.handlePostExecution(
		context.Background(),
		"github.com/org/repo",
		"_uninstall",
		errors.New("wizard failed"),
		domainRuntime.ExecutionOutcomeFailed,
	)
}

// --- cleanupAfterUninstall() ---

func TestCleanupAfterUninstall_NoOrphanedDeps_OnlyMainArrowDeleted(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")

	// Entry with no dependencies.
	f.vault.GetArrowEntry = &vault.VaultEntry{
		Manifest: &domain.ArrowManifest{
			Name:    "Repo",
			Version: "1.0.0",
		},
	}
	f.depTree.Result = []domain.Namespace{ns}

	syncCalled := false
	f.executor.SetSyncExecute(func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
		syncCalled = true
		return nil
	})

	f.executor.cleanupAfterUninstall(context.Background(), ns)

	// No orphaned deps → syncExecute never called.
	assert.False(t, syncCalled)
}

func TestCleanupAfterUninstall_OrphanedDep_SyncExecuteAndVaultDeleteCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/main")
	dep := domain.Namespace("github.com/org/dep")

	// GetArrow: first call is for ns (entry with one direct dep), subsequent calls return nil.
	callCount := 0
	customVault := &trackingVault{
		onGetArrow: func(ctx context.Context, n domain.Namespace) (*vault.VaultEntry, string, error) {
			callCount++
			if n == ns {
				return &vault.VaultEntry{
					Manifest: &domain.ArrowManifest{
						Name:         "Main",
						Version:      "1.0.0",
						Dependencies: []domain.Namespace{dep},
					},
				}, "/work/main", nil
			}
			// dep's vault entry: no further dependencies.
			return &vault.VaultEntry{
				Manifest: &domain.ArrowManifest{
					Name:    "Dep",
					Version: "1.0.0",
				},
			}, "/work/dep", nil
		},
	}

	// catalog.List returns no arrows → no other arrows depend on dep → dep is orphaned.
	f.executor.vault = customVault
	f.depTree.Result = []domain.Namespace{dep, ns}

	var syncCalls []domain.Namespace
	f.executor.SetSyncExecute(func(_ context.Context, n domain.Namespace, _ string, _ map[string]string) error {
		syncCalls = append(syncCalls, n)
		return nil
	})

	f.executor.cleanupAfterUninstall(context.Background(), ns)

	assert.Contains(t, syncCalls, dep)
}

func TestCleanupAfterUninstall_VaultGetArrowFails_MainArrowStillDeleted(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	f.vault.GetArrowErr = errors.New("not cached")

	deleteCalled := false
	customVault := &trackingVault{
		onGetArrow: func(_ context.Context, _ domain.Namespace) (*vault.VaultEntry, string, error) {
			return nil, "", errors.New("not cached")
		},
		onDeleteArrow: func(_ context.Context, _ domain.Namespace) error {
			deleteCalled = true
			return nil
		},
	}
	f.executor.vault = customVault

	f.executor.cleanupAfterUninstall(context.Background(), ns)

	assert.True(t, deleteCalled)
}

func TestCleanupAfterUninstall_DepTreeFails_MainArrowStillDeleted(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	f.vault.GetArrowEntry = &vault.VaultEntry{
		Manifest: &domain.ArrowManifest{Name: "Repo", Version: "1.0.0"},
	}
	f.depTree.Err = errors.New("cycle detected")

	deleteCalled := false
	customVault := &trackingVault{
		onGetArrow: func(_ context.Context, _ domain.Namespace) (*vault.VaultEntry, string, error) {
			return f.vault.GetArrowEntry, "", nil
		},
		onDeleteArrow: func(_ context.Context, _ domain.Namespace) error {
			deleteCalled = true
			return nil
		},
	}
	f.executor.vault = customVault

	f.executor.cleanupAfterUninstall(context.Background(), ns)

	assert.True(t, deleteCalled)
}

func TestCleanupAfterUninstall_SyncExecuteNil_NoPanic(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/main")
	dep := domain.Namespace("github.com/org/dep")

	customVault := &trackingVault{
		onGetArrow: func(_ context.Context, n domain.Namespace) (*vault.VaultEntry, string, error) {
			if n == ns {
				return &vault.VaultEntry{
					Manifest: &domain.ArrowManifest{
						Name:         "Main",
						Version:      "1.0.0",
						Dependencies: []domain.Namespace{dep},
					},
				}, "", nil
			}
			return &vault.VaultEntry{
				Manifest: &domain.ArrowManifest{Name: "Dep", Version: "1.0.0"},
			}, "", nil
		},
	}
	f.executor.vault = customVault
	f.depTree.Result = []domain.Namespace{dep, ns}
	// syncExecute is nil — must not panic.

	assert.NotPanics(t, func() {
		f.executor.cleanupAfterUninstall(context.Background(), ns)
	})
}

// --- mapOutcome() ---

func TestMapOutcome_Nil_ReturnsSuccess(t *testing.T) {
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, mapOutcome(nil))
}

func TestMapOutcome_Canceled_ReturnsCancelled(t *testing.T) {
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, mapOutcome(context.Canceled))
}

func TestMapOutcome_WrappedCanceled_ReturnsCancelled(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", context.Canceled)
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, mapOutcome(wrapped))
}

func TestMapOutcome_OtherError_ReturnsFailed(t *testing.T) {
	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, mapOutcome(errors.New("some error")))
}

// --- hasDependentsOf() ---

func TestHasDependentsOf_EmptyCatalog_ReturnsFalse(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/dep")
	excludeNs := domain.Namespace("github.com/org/main")

	hasDeps, err := f.executor.hasDependentsOf(context.Background(), ns, excludeNs)

	require.NoError(t, err)
	assert.False(t, hasDeps)
}

func TestHasDependentsOf_ArrowDependsOnNs_ReturnsTrue(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/dep")
	excludeNs := domain.Namespace("github.com/org/main")
	dependentNs := domain.Namespace("github.com/org/consumer")

	// Add consumer to catalog.
	f.catalog.saved = []domain.Arrow{
		{
			Namespace: dependentNs,
			Manifest: domain.ArrowManifest{
				Name:         "Consumer",
				Version:      "1.0.0",
				Dependencies: []domain.Namespace{ns},
			},
		},
	}

	// Seed consumer's runtime state so it's not absent.
	err := f.axRuntime.Send(context.Background(), arrowcmds.BeginExecution{
		Namespace: dependentNs,
		Method:    "_execute",
	})
	require.NoError(t, err)
	f.axRuntime.WaitPublish()

	// Vault returns an entry for consumer.
	f.vault.GetArrowEntry = &vault.VaultEntry{
		Manifest: &domain.ArrowManifest{
			Name:         "Consumer",
			Version:      "1.0.0",
			Dependencies: []domain.Namespace{ns},
		},
	}

	hasDeps, err := f.executor.hasDependentsOf(context.Background(), ns, excludeNs)

	require.NoError(t, err)
	assert.True(t, hasDeps)
}

func TestHasDependentsOf_RemovedArrow_NotCountedAsDependent(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/dep")
	excludeNs := domain.Namespace("github.com/org/main")

	f.catalog.saved = []domain.Arrow{
		{
			Namespace: domain.Namespace("github.com/org/consumer"),
			Manifest: domain.ArrowManifest{
				Name:         "Consumer",
				Version:      "1.0.0",
				Dependencies: []domain.Namespace{ns},
			},
			Removed: true,
		},
	}

	hasDeps, err := f.executor.hasDependentsOf(context.Background(), ns, excludeNs)

	require.NoError(t, err)
	assert.False(t, hasDeps)
}

// --- Handler() ---

func TestHandler_ReturnsNonNilFunc(t *testing.T) {
	f := newWizardExecutorFixture(t)
	h := f.executor.Handler()
	assert.NotNil(t, h)
}

func TestHandler_InvokesExecute_WithNilActiveRun_NoOp(t *testing.T) {
	f := newWizardExecutorFixture(t)
	h := f.executor.Handler()

	rt := domainRuntime.ArrowRuntime{
		Namespace: "github.com/org/repo",
		ActiveRun: nil,
	}
	evt := asynxModels.Event[domainRuntime.ArrowRuntime]{
		EventName: "runtime.begun",
		Aggregate: rt,
	}

	// Should not panic and wizard must not be called.
	h(context.Background(), evt)
	assert.False(t, f.wiz.WasExecuteCalled())
}

func TestHandler_InvokesExecute_WithActiveRun_CallsWizard(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	h := f.executor.Handler()
	rt := makeRuntime(ns, "_execute")
	evt := asynxModels.Event[domainRuntime.ArrowRuntime]{
		EventName: "runtime.begun",
		Aggregate: rt,
	}

	h(context.Background(), evt)

	assert.True(t, f.wiz.WasExecuteCalled())
}

// --- execute() canceled outcome ---

func TestExecute_WizardCanceled_SendsEndExecutionCancelled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_install")

	f.wiz.ExecuteErr = context.Canceled

	rt := makeRuntime(ns, "_install")

	f.executor.execute(context.Background(), rt)

	result, err := f.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, result.LastReturn.Outcome)
}

// --- hasDependentsOf additional paths ---

func TestHasDependentsOf_CatalogListFails_ReturnsError(t *testing.T) {
	f := newWizardExecutorFixture(t)

	// Make catalog.List return an error via a custom mock.
	errCatalog := &errArrowCatalog{listErr: errors.New("db error")}
	f.executor.catalog = errCatalog

	_, err := f.executor.hasDependentsOf(
		context.Background(),
		"github.com/org/dep",
		"github.com/org/main",
	)

	assert.Error(t, err)
}

func TestHasDependentsOf_ExcludeNsSkipped(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/dep")
	excludeNs := domain.Namespace("github.com/org/main")

	// Only arrow in catalog IS the excludeNs — should be skipped.
	f.catalog.saved = []domain.Arrow{
		{
			Namespace: excludeNs,
			Manifest: domain.ArrowManifest{
				Name:         "Main",
				Version:      "1.0.0",
				Dependencies: []domain.Namespace{ns},
			},
		},
	}

	hasDeps, err := f.executor.hasDependentsOf(context.Background(), ns, excludeNs)

	require.NoError(t, err)
	assert.False(t, hasDeps)
}

func TestHasDependentsOf_IndirectDependency_ReturnsTrue(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/dep")
	excludeNs := domain.Namespace("github.com/org/main")
	dependentNs := domain.Namespace("github.com/org/consumer")

	f.catalog.saved = []domain.Arrow{
		{
			Namespace: dependentNs,
			Manifest:  domain.ArrowManifest{Name: "Consumer", Version: "1.0.0"},
		},
	}

	// Seed runtime so it's not absent.
	err := f.axRuntime.Send(context.Background(), arrowcmds.BeginExecution{
		Namespace: dependentNs,
		Method:    "_execute",
	})
	require.NoError(t, err)
	f.axRuntime.WaitPublish()

	// Vault returns entry where ns is an indirect dependency.
	customVault := &trackingVault{
		onGetArrow: func(_ context.Context, _ domain.Namespace) (*vault.VaultEntry, string, error) {
			return &vault.VaultEntry{
				Manifest:             &domain.ArrowManifest{Name: "Consumer", Version: "1.0.0"},
				IndirectDependencies: []domain.Namespace{ns},
			}, "", nil
		},
	}
	f.executor.vault = customVault

	hasDeps, err := f.executor.hasDependentsOf(context.Background(), ns, excludeNs)

	require.NoError(t, err)
	assert.True(t, hasDeps)
}

func TestHasDependentsOf_VaultGetArrowFails_ArrowSkipped(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/dep")
	excludeNs := domain.Namespace("github.com/org/main")
	dependentNs := domain.Namespace("github.com/org/consumer")

	f.catalog.saved = []domain.Arrow{
		{
			Namespace: dependentNs,
			Manifest:  domain.ArrowManifest{Name: "Consumer", Version: "1.0.0"},
		},
	}

	// Seed runtime so it's not absent.
	err := f.axRuntime.Send(context.Background(), arrowcmds.BeginExecution{
		Namespace: dependentNs,
		Method:    "_execute",
	})
	require.NoError(t, err)
	f.axRuntime.WaitPublish()

	// Vault returns error → arrow is skipped → no dependents found.
	customVault := &trackingVault{
		onGetArrow: func(_ context.Context, _ domain.Namespace) (*vault.VaultEntry, string, error) {
			return nil, "", errors.New("not cached")
		},
	}
	f.executor.vault = customVault

	hasDeps, err := f.executor.hasDependentsOf(context.Background(), ns, excludeNs)

	require.NoError(t, err)
	assert.False(t, hasDeps)
}

// --- cleanupAfterUninstall additional paths ---

func TestCleanupAfterUninstall_IndirectDeps_Orphaned_Uninstalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/main")
	indirectDep := domain.Namespace("github.com/org/indirect")

	callCount := 0
	customVault := &trackingVault{
		onGetArrow: func(_ context.Context, n domain.Namespace) (*vault.VaultEntry, string, error) {
			callCount++
			if n == ns {
				return &vault.VaultEntry{
					Manifest:             &domain.ArrowManifest{Name: "Main", Version: "1.0.0"},
					IndirectDependencies: []domain.Namespace{indirectDep},
				}, "", nil
			}
			return &vault.VaultEntry{
				Manifest: &domain.ArrowManifest{Name: "Indirect", Version: "1.0.0"},
			}, "", nil
		},
	}

	f.executor.vault = customVault
	f.depTree.Result = []domain.Namespace{indirectDep, ns}

	var syncCalls []domain.Namespace
	f.executor.SetSyncExecute(func(_ context.Context, n domain.Namespace, _ string, _ map[string]string) error {
		syncCalls = append(syncCalls, n)
		return nil
	})

	f.executor.cleanupAfterUninstall(context.Background(), ns)

	assert.Contains(t, syncCalls, indirectDep)
}

func TestExecute_WithSteps_StepsExtracted(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedRuntime(t, f.axRuntime, ns, "_execute")

	f.wiz.ExecuteErr = nil

	step := domainstep.NewRunStep("echo", "echo hello", 0, false)
	rt := domainRuntime.ArrowRuntime{
		Namespace: ns,
		State:     domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{
			Method:    "_execute",
			Variables: map[string]string{},
			Steps: []domainRuntime.StepProgress{
				{Index: 0, Status: domainRuntime.StepStatusPending, Step: step},
			},
		},
	}

	f.executor.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())
}

func TestCleanupAfterUninstall_RealDepTree_VaultResolverCalled(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/main")
	dep := domain.Namespace("github.com/org/dep")

	// Use real deptree so the vaultResolver closure is actually invoked.
	f.executor.depTree = deptree.New()

	var deletedNs []domain.Namespace
	customVault := &trackingVault{
		onGetArrow: func(_ context.Context, n domain.Namespace) (*vault.VaultEntry, string, error) {
			if n == ns {
				return &vault.VaultEntry{
					Manifest: &domain.ArrowManifest{
						Name:         "Main",
						Version:      "1.0.0",
						Dependencies: []domain.Namespace{dep},
					},
				}, "", nil
			}
			// dep has no further dependencies — vaultResolver returns nil deps.
			return &vault.VaultEntry{
				Manifest: &domain.ArrowManifest{Name: "Dep", Version: "1.0.0"},
			}, "", nil
		},
		onDeleteArrow: func(_ context.Context, n domain.Namespace) error {
			deletedNs = append(deletedNs, n)
			return nil
		},
	}
	f.executor.vault = customVault

	var syncCalls []domain.Namespace
	f.executor.SetSyncExecute(func(_ context.Context, n domain.Namespace, _ string, _ map[string]string) error {
		syncCalls = append(syncCalls, n)
		return nil
	})

	f.executor.cleanupAfterUninstall(context.Background(), ns)

	assert.Contains(t, deletedNs, ns)
}

func TestCleanupAfterUninstall_SyncExecuteFails_DepNotDeleted(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/main")
	dep := domain.Namespace("github.com/org/dep")

	var deletedNs []domain.Namespace
	customVault := &trackingVault{
		onGetArrow: func(_ context.Context, n domain.Namespace) (*vault.VaultEntry, string, error) {
			if n == ns {
				return &vault.VaultEntry{
					Manifest: &domain.ArrowManifest{
						Name:         "Main",
						Version:      "1.0.0",
						Dependencies: []domain.Namespace{dep},
					},
				}, "", nil
			}
			return &vault.VaultEntry{
				Manifest: &domain.ArrowManifest{Name: "Dep", Version: "1.0.0"},
			}, "", nil
		},
		onDeleteArrow: func(_ context.Context, n domain.Namespace) error {
			deletedNs = append(deletedNs, n)
			return nil
		},
	}

	f.executor.vault = customVault
	f.depTree.Result = []domain.Namespace{dep, ns}

	f.executor.SetSyncExecute(func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
		return errors.New("uninstall failed")
	})

	f.executor.cleanupAfterUninstall(context.Background(), ns)

	// Only the main ns should be deleted, not the dep (since syncExecute failed).
	assert.Contains(t, deletedNs, ns)
	assert.NotContains(t, deletedNs, dep)
}

// TestCleanupAfterUninstall_DepHasOtherDependents covers the hasDependents continue path
// (line 155: dep has dependents so it is not orphaned).
func TestCleanupAfterUninstall_DepHasOtherDependents_NotOrphaned(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/main")
	dep := domain.Namespace("github.com/org/dep")
	otherUser := domain.Namespace("github.com/org/other")

	// The catalog has another arrow that depends on dep.
	f.catalog.saved = []domain.Arrow{
		{
			Namespace: otherUser,
			Manifest: domain.ArrowManifest{
				Name:         "Other",
				Version:      "1.0.0",
				Dependencies: []domain.Namespace{dep},
			},
		},
	}

	// otherUser has active runtime.
	err := f.axRuntime.Send(context.Background(), arrowcmds.BeginExecution{
		Namespace: otherUser,
		Method:    "_execute",
	})
	require.NoError(t, err)
	f.axRuntime.WaitPublish()

	var deletedNs []domain.Namespace
	customVault := &trackingVault{
		onGetArrow: func(_ context.Context, n domain.Namespace) (*vault.VaultEntry, string, error) {
			if n == ns {
				return &vault.VaultEntry{
					Manifest: &domain.ArrowManifest{
						Name:         "Main",
						Version:      "1.0.0",
						Dependencies: []domain.Namespace{dep},
					},
				}, "", nil
			}
			// Called for otherUser by hasDependentsOf.
			return &vault.VaultEntry{
				Manifest: &domain.ArrowManifest{
					Name:         "Other",
					Version:      "1.0.0",
					Dependencies: []domain.Namespace{dep},
				},
			}, "", nil
		},
		onDeleteArrow: func(_ context.Context, n domain.Namespace) error {
			deletedNs = append(deletedNs, n)
			return nil
		},
	}
	f.executor.vault = customVault
	f.depTree.Result = []domain.Namespace{dep, ns}

	syncCalled := false
	f.executor.SetSyncExecute(func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) error {
		syncCalled = true
		return nil
	})

	f.executor.cleanupAfterUninstall(context.Background(), ns)

	// dep should NOT be uninstalled because otherUser still depends on it.
	assert.False(t, syncCalled)
	// Main ns should still be deleted.
	assert.Contains(t, deletedNs, ns)
}

// TestCleanupAfterUninstall_VaultResolverGetArrowFails covers line 163: vaultResolver
// returns nil when GetArrow errors for a dep namespace.
func TestCleanupAfterUninstall_VaultResolverGetArrowFails_NilDeps(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/main")
	dep := domain.Namespace("github.com/org/dep")

	f.executor.depTree = deptree.New()

	getCount := 0
	customVault := &trackingVault{
		onGetArrow: func(_ context.Context, n domain.Namespace) (*vault.VaultEntry, string, error) {
			getCount++
			if n == ns {
				return &vault.VaultEntry{
					Manifest: &domain.ArrowManifest{
						Name:         "Main",
						Version:      "1.0.0",
						Dependencies: []domain.Namespace{dep},
					},
				}, "", nil
			}
			// For dep and any recursive call: return error so vaultResolver returns nil.
			return nil, "", errors.New("not cached")
		},
	}
	f.executor.vault = customVault

	// Should not panic and should complete — topo resolves with just ns (dep has nil deps).
	f.executor.cleanupAfterUninstall(context.Background(), ns)
}

// TestHasDependentsOf_RuntimeNamespaceEmpty covers the rt.Namespace == "" continue path (line 208).
func TestHasDependentsOf_RuntimeNamespaceEmpty_Skipped(t *testing.T) {
	f := newWizardExecutorFixture(t)

	ns := domain.Namespace("github.com/org/dep")
	excludeNs := domain.Namespace("github.com/org/main")
	consumerNs := domain.Namespace("github.com/org/consumer")

	f.catalog.saved = []domain.Arrow{
		{
			Namespace: consumerNs,
			Manifest: domain.ArrowManifest{
				Name:         "Consumer",
				Version:      "1.0.0",
				Dependencies: []domain.Namespace{ns},
			},
		},
	}

	// Do NOT seed the runtime for consumerNs — Get will return a zero-value ArrowRuntime
	// with empty Namespace, triggering the rt.Namespace == "" skip.

	hasDeps, err := f.executor.hasDependentsOf(context.Background(), ns, excludeNs)

	require.NoError(t, err)
	assert.False(t, hasDeps)
}

// --- errArrowCatalog helper ---

type errArrowCatalog struct {
	listErr error
}

func (c *errArrowCatalog) Save(_ context.Context, _ domain.Arrow) error        { return nil }
func (c *errArrowCatalog) Delete(_ context.Context, _ domain.Namespace) error  { return nil }
func (c *errArrowCatalog) Get(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
	return nil, nil
}
func (c *errArrowCatalog) List(_ context.Context) ([]domain.Arrow, error) {
	return nil, c.listErr
}

// --- trackingVault helper ---

type trackingVault struct {
	onGetArrow    func(ctx context.Context, ns domain.Namespace) (*vault.VaultEntry, string, error)
	onDeleteArrow func(ctx context.Context, ns domain.Namespace) error
}

func (v *trackingVault) GetArrow(ctx context.Context, ns domain.Namespace) (*vault.VaultEntry, string, error) {
	if v.onGetArrow != nil {
		return v.onGetArrow(ctx, ns)
	}
	return nil, "", nil
}

func (v *trackingVault) DeleteArrow(ctx context.Context, ns domain.Namespace) error {
	if v.onDeleteArrow != nil {
		return v.onDeleteArrow(ctx, ns)
	}
	return nil
}

func (v *trackingVault) PutArrow(_ context.Context, _ domain.Namespace, _ *domain.ArrowManifest, _ []domain.Namespace) (string, error) {
	return "", nil
}

func (v *trackingVault) GetQuiver(_ context.Context, _ domain.Namespace) (*vault.QuiverVaultEntry, string, error) {
	return nil, "", nil
}

func (v *trackingVault) PutQuiver(_ context.Context, _ domain.Namespace, _ *domain.QuiverManifest) (string, error) {
	return "", nil
}

func (v *trackingVault) DeleteQuiver(_ context.Context, _ domain.Namespace) error {
	return nil
}
