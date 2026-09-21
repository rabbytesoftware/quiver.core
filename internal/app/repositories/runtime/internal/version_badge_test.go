package runtimeinternal_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoRuntime "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	runtimeinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

// seedDriftedRunningRuntime brings a runtime to ready, marks it outdated the
// way a version check does, and then starts an execute on it. That is exactly
// the state a user puts a drifted arrow into by running it: BeginExecution
// admits the version-drift flavour of Outdated wherever it admits Ready.
func seedDriftedRunningRuntime(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
) {
	t.Helper()
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.MarkVersionOutdated{Namespace: ns})
	require.NoError(t, err)

	current, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.Equal(t, domain.ArrowStateOutdated, current.State,
		"the seed must start from a genuinely drifted arrow")

	_, err = ax.Send(context.Background(), commands.BeginExecution{
		Namespace:   ns,
		ExecutionID: testExecutionID,
		Method:      domain.MethodExecute,
		Steps:       domainStep.StepList{testStep()},
	})
	require.NoError(t, err)
}

// catalogSaying builds the real production reconcile over a catalog that
// answers `outdated` for every namespace. Using repoRuntime.ReconcileVersionBadge
// rather than a stub is the point: what is under test is the whole path from
// the catalog fact to the state the badge reads, not the drain's willingness to
// call a closure.
func catalogSaying(
	outdated bool,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
) func(ctx context.Context, ns domain.Namespace) error {
	return repoRuntime.ReconcileVersionBadge(
		func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
			return &domain.Arrow{Namespace: ns, Outdated: outdated}, nil
		},
		ax,
	)
}

// TestDrainExecution_DriftedArrow_IsOutdatedAgainBeforeTheDrainReturns is the
// badge-window regression. EndExecution rewrites the state from the method and
// outcome alone, so before this fix a drifted arrow came out of a run reading
// Ready — and the list and WebSocket views, which read ArrowRuntime.State, showed
// no badge until the next TTL-gated version check, up to an hour later.
//
// The assertion deliberately takes no wait of any kind: DrainExecution is called
// on this goroutine and the state is read the instant it returns. If a window
// existed at all, this would see Ready.
func TestDrainExecution_DriftedArrow_IsOutdatedAgainBeforeTheDrainReturns(t *testing.T) {
	ns := domain.Namespace("github.com/user/drifted@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedDriftedRunningRuntime(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute,
		noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime,
		catalogSaying(true, axRuntime),
	)

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State,
		"an arrow the catalog still calls outdated must read outdated the moment its run ends")
	assert.Nil(t, got.Execution, "the execution is still over")
	require.NotNil(t, got.LastReturn, "and the run's return is still readable")
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, got.LastReturn.Outcome)
}

// A failed run is no evidence about whether a newer release exists, so the
// badge comes back either way.
func TestDrainExecution_DriftedArrow_FailedRun_StillOutdated(t *testing.T) {
	ns := domain.Namespace("github.com/user/driftedfail@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedDriftedRunningRuntime(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeFailed)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute,
		noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime,
		catalogSaying(true, axRuntime),
	)

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State)
}

// The ordinary case — almost every execution in the system — must be exactly
// as it was: an arrow with no drift ends its run at Ready and stays there.
func TestDrainExecution_UndriftedArrow_EndsReadyAsBefore(t *testing.T) {
	ns := domain.Namespace("github.com/user/current@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute,
		noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime,
		catalogSaying(false, axRuntime),
	)

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

// A successful uninstall leaves the arrow absent. The catalog row may well
// still carry Outdated from before it was removed, and the badge reconcile must
// not resurrect a runtime state for software that is no longer installed — the
// command's own state guard is what refuses it.
func TestDrainExecution_UninstallSuccess_DriftedCatalog_StaysAbsent(t *testing.T) {
	ns := domain.Namespace("github.com/user/goneanyway@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedUninstallingRuntimeForHooks(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodUninstall,
		noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime,
		catalogSaying(true, axRuntime),
	)

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, got.State)
}

// A superseded end describes a run nobody is waiting on — the aggregate has
// already moved to another execution. Reconciling the badge off the back of it
// would read a state that belongs to the takeover, so the reconcile must not
// run at all.
func TestDrainExecution_SupersededEnd_DoesNotReconcileBadge(t *testing.T) {
	ns := domain.Namespace("github.com/user/superseded@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedDriftedRunningRuntime(t, axRuntime, ns)

	// A stop takes the arrow over mid-run; the drained execution's end is no
	// longer the aggregate's current one.
	_, err := axRuntime.Send(context.Background(), commands.BeginStop{
		Namespace:   ns,
		ExecutionID: "exec-2",
		Steps:       domainStep.StepList{testStep()},
	})
	require.NoError(t, err)

	called := atomic.Bool{}
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute,
		noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime,
		func(_ context.Context, _ domain.Namespace) error {
			called.Store(true)
			return nil
		},
	)

	assert.False(t, called.Load(),
		"a superseded end must not reconcile the badge of the execution that replaced it")
}

// A reconcile that fails is logged, not fatal: the next version check puts the
// badge right, exactly as it did before this path existed.
func TestDrainExecution_ReconcileError_LeavesTheEndCommitted(t *testing.T) {
	ns := domain.Namespace("github.com/user/reconcilefails@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedDriftedRunningRuntime(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute,
		noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime,
		func(_ context.Context, _ domain.Namespace) error { return assert.AnError },
	)

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
	assert.Nil(t, got.Execution)
}

// A container built without the hook — every caller that has no catalog to ask
// — must drain exactly as it always did.
func TestDrainExecution_NoReconcileHook_EndsReady(t *testing.T) {
	ns := domain.Namespace("github.com/user/nohook@v1.0.0")
	axRuntime := newTestAsynxRuntimeForHooks(t)
	seedDriftedRunningRuntime(t, axRuntime, ns)

	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(
		context.Background(), exec, ns.String(), testExecutionID, domain.MethodExecute,
		noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, axRuntime,
	)

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}
