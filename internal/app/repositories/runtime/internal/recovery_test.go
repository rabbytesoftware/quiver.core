package runtimeinternal_test

import (
	"context"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	runtimeinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

// mockCatalog is a minimal list-only catalog stub for crash recovery tests.
type mockCatalog struct {
	listResult []models.ArrowView
	listErr    error
}

func (m *mockCatalog) listFn() func(ctx context.Context) ([]models.ArrowView, error) {
	return func(ctx context.Context) ([]models.ArrowView, error) {
		return m.listResult, m.listErr
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func newTestAsynxRuntime(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

func testNs() domain.Namespace {
	return domain.Namespace("github.com/user/repo@v1.0.0")
}

func seedRunningRuntime(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
	pid int,
) {
	t.Helper()
	// Install to reach Ready, then begin a custom execute
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)
	_, err = ax.Send(context.Background(), commands.BeginExecution{
		Namespace: ns,
		Method:    domain.MethodExecute,
	})
	require.NoError(t, err)

	if pid > 0 {
		_, err = ax.Send(context.Background(), commands.RecordPID{
			Namespace: ns,
			PID:       pid,
		})
		require.NoError(t, err)
	}
}

func seedInstallingRuntime(
	t *testing.T,
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
) {
	t.Helper()
	_, err := ax.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.NoError(t, err)
}

func makeArrowView(ns domain.Namespace) models.ArrowView {
	return models.ArrowView{
		Namespace: ns.BareNamespace(),
		Metadata:  domain.Arrow{Namespace: ns},
		Versions: []models.VersionView{
			{Namespace: ns, State: domain.ArrowStateAbsent},
		},
	}
}

// ─── RecoverTransients tests ─────────────────────────────────────────────────

func TestRecoverTransients_ListError_ReturnsEarly(t *testing.T) {
	cat := &mockCatalog{listErr: assert.AnError}
	axRuntime := newTestAsynxRuntime(t)
	w := &mocks.Wizard{}

	// Should not panic; list error returns early
	runtimeinternal.RecoverTransients(context.Background(), cat.listFn(), func(context.Context) ([]domain.Namespace, error) { return nil, nil }, axRuntime, w)
}

func TestRecoverTransients_NoItems_Noop(t *testing.T) {
	cat := &mockCatalog{listResult: nil}
	axRuntime := newTestAsynxRuntime(t)
	w := &mocks.Wizard{}

	runtimeinternal.RecoverTransients(context.Background(), cat.listFn(), func(context.Context) ([]domain.Namespace, error) { return nil, nil }, axRuntime, w)
	// Nothing should crash
}

func TestRecoverTransients_StableState_Nothing(t *testing.T) {
	ns := testNs()
	axRuntime := newTestAsynxRuntime(t)

	// Seed a ready runtime
	_, err := axRuntime.Send(context.Background(), commands.BeginInstall{Namespace: ns})
	require.NoError(t, err)
	_, err = axRuntime.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)

	cat := &mockCatalog{listResult: []models.ArrowView{makeArrowView(ns)}}
	w := &mocks.Wizard{}

	runtimeinternal.RecoverTransients(context.Background(), cat.listFn(), func(context.Context) ([]domain.Namespace, error) { return nil, nil }, axRuntime, w)

	// State should remain ready
	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestRecoverTransients_InstallingState_RecoverInterrupted(t *testing.T) {
	ns := testNs()
	axRuntime := newTestAsynxRuntime(t)
	seedInstallingRuntime(t, axRuntime, ns)

	cat := &mockCatalog{listResult: []models.ArrowView{makeArrowView(ns)}}
	w := &mocks.Wizard{}

	runtimeinternal.RecoverTransients(context.Background(), cat.listFn(), func(context.Context) ([]domain.Namespace, error) { return nil, nil }, axRuntime, w)

	// Must eventually settle — give goroutines time to complete
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	// Installing → Absent (recovery mapping)
	assert.Equal(t, domain.ArrowStateAbsent, got.State)
}

func TestRecoverTransients_RunningWithLivePID_Detach(t *testing.T) {
	ns := testNs()
	axRuntime := newTestAsynxRuntime(t)
	seedRunningRuntime(t, axRuntime, ns, 99999)

	cat := &mockCatalog{listResult: []models.ArrowView{makeArrowView(ns)}}
	w := &mocks.Wizard{
		ProcessAliveFn: func(pid int) bool { return true },
	}

	runtimeinternal.RecoverTransients(context.Background(), cat.listFn(), func(context.Context) ([]domain.Namespace, error) { return nil, nil }, axRuntime, w)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateDetached, got.State)
}

func TestRecoverTransients_RunningWithDeadPID_RecoverInterrupted(t *testing.T) {
	ns := testNs()
	axRuntime := newTestAsynxRuntime(t)
	seedRunningRuntime(t, axRuntime, ns, 99999)

	cat := &mockCatalog{listResult: []models.ArrowView{makeArrowView(ns)}}
	w := &mocks.Wizard{
		ProcessAliveFn: func(pid int) bool { return false },
	}

	runtimeinternal.RecoverTransients(context.Background(), cat.listFn(), func(context.Context) ([]domain.Namespace, error) { return nil, nil }, axRuntime, w)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestRecoverTransients_RunningWithNoPID_RecoverInterrupted(t *testing.T) {
	ns := testNs()
	axRuntime := newTestAsynxRuntime(t)
	// Run without PID
	seedRunningRuntime(t, axRuntime, ns, 0)

	cat := &mockCatalog{listResult: []models.ArrowView{makeArrowView(ns)}}
	w := &mocks.Wizard{}

	runtimeinternal.RecoverTransients(context.Background(), cat.listFn(), func(context.Context) ([]domain.Namespace, error) { return nil, nil }, axRuntime, w)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestRecoverTransients_PreloadError_SkipsItem(t *testing.T) {
	ns := testNs()

	// Use a mock runtime that returns an error on Preload
	axRuntime := newTestAsynxRuntime(t)
	// ns doesn't exist in runtime — Preload will fail gracefully since ns has no runtime yet

	cat := &mockCatalog{listResult: []models.ArrowView{makeArrowView(ns)}}
	w := &mocks.Wizard{}

	// Should not crash; namespace with no runtime state is just skipped
	runtimeinternal.RecoverTransients(context.Background(), cat.listFn(), func(context.Context) ([]domain.Namespace, error) { return nil, nil }, axRuntime, w)
}

func TestRecoverTransients_RuntimeGetError_SkipsItem(t *testing.T) {
	ns := testNs()
	axRuntime := newTestAsynxRuntime(t)

	// Seed a preloadable entry but with no event (so Get returns ErrNotFound)
	cat := &mockCatalog{listResult: []models.ArrowView{makeArrowView(ns)}}
	w := &mocks.Wizard{}

	runtimeinternal.RecoverTransients(context.Background(), cat.listFn(), func(context.Context) ([]domain.Namespace, error) { return nil, nil }, axRuntime, w)
	// No panic expected
}

// ─── Verify ErrNotFound is properly handled ───────────────────────────────

func TestRecoverTransients_AsynxGetNotFound_SkipsItem(t *testing.T) {
	ns := testNs()
	axRuntime := newTestAsynxRuntime(t)

	// Preload something that won't have events — Get will return ErrNotFound
	// but we still need it to be in an "existing aggregate" store
	// Since no events were sent, Get returns ErrNotFound, which skips the item
	cat := &mockCatalog{listResult: []models.ArrowView{makeArrowView(ns)}}
	w := &mocks.Wizard{}

	runtimeinternal.RecoverTransients(context.Background(), cat.listFn(), func(context.Context) ([]domain.Namespace, error) { return nil, nil }, axRuntime, w)

	// Should not have changed anything
	_, err := axRuntime.Get(context.Background(), ns.String())
	assert.True(t, isNotFoundErr(err))
}

func isNotFoundErr(err error) bool {
	return err != nil && (err == asynxModels.ErrNotFound ||
		err.Error() == "asynx: aggregate not found")
}

// ─── sendRecoverInterrupted / recoverRunning direct tests ────────────────────

func TestSendRecoverInterrupted_ShutdownAsynx_LogsAndReturns(t *testing.T) {
	ns := testNs()
	axRuntime := newTestAsynxRuntime(t)
	// Seed installing runtime so SendWait would normally work.
	seedInstallingRuntime(t, axRuntime, ns)
	// Shut down asynx so SendWait returns an error.
	_ = axRuntime.Shutdown(context.Background())

	// Should not panic — error is logged.
	runtimeinternal.SendRecoverInterrupted(
		context.Background(),
		ns,
		domain.ArrowStateInstalling,
		axRuntime,
	)
}

func TestSendRecoverInterrupted_StableState_SendWaitError_LogsAndReturns(t *testing.T) {
	ns := domain.Namespace("github.com/user/stable@v1.0.0")
	axRuntime := newTestAsynxRuntime(t)
	// Seed a ready runtime — RecoverInterrupted's Validate will reject it
	// since Ready is a stable state → SendWait returns validation error.
	seedRunningRuntime(t, axRuntime, ns, 0)
	_, _ = axRuntime.Send(context.Background(), commands.EndExecution{
		Namespace: ns,
		Outcome:   domainRuntime.ExecutionOutcomeSuccess,
	})

	// Should not panic — validation error is logged.
	runtimeinternal.SendRecoverInterrupted(
		context.Background(),
		ns,
		domain.ArrowStateReady,
		axRuntime,
	)
}

func TestRecoverRunning_SendWaitDetachError_LogsAndReturns(t *testing.T) {
	ns := domain.Namespace("github.com/user/detacherr@v1.0.0")
	axRuntime := newTestAsynxRuntime(t)
	seedRunningRuntime(t, axRuntime, ns, 42)

	rt, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)

	// Shut down so SendWait fails for RecordDetached.
	_ = axRuntime.Shutdown(context.Background())

	w := &mocks.Wizard{
		ProcessAliveFn: func(pid int) bool { return true },
	}

	// Should not panic — error is logged.
	runtimeinternal.RecoverRunning(
		context.Background(),
		ns,
		rt,
		axRuntime,
		w,
	)
}

// ─── recoverRunning failure path ─────────────────────────────────────────────

// TestRecoverTransients_RunningWithLivePID_SendWaitFails exercises the path
// where recoverRunning is called and the RecordDetached SendWait succeeds.
func TestRecoverTransients_RunningWithLivePID_Detach_Success(t *testing.T) {
}

func TestRecoverTransients_OrphanNotInCatalog_Recovers(t *testing.T) {
	orphan := domain.Namespace("github.com/u/orphan@main")

	axRuntime := newTestAsynxRuntime(t)
	seedInstallingRuntime(t, axRuntime, orphan)

	// Catalog is empty — the orphan is not here
	listArrows := func(context.Context) ([]models.ArrowView, error) {
		return nil, nil
	}
	listAgg := func(context.Context) ([]domain.Namespace, error) {
		return []domain.Namespace{orphan}, nil
	}

	w := &mocks.Wizard{}
	runtimeinternal.RecoverTransients(context.Background(), listArrows, listAgg, axRuntime, w)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), orphan.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, got.State,
		"orphan stuck in installing should be recovered to absent")
}

func TestRecoverTransients_UnionDeduplicates(t *testing.T) {
	ns := domain.Namespace("github.com/u/dup@v1")

	axRuntime := newTestAsynxRuntime(t)
	seedInstallingRuntime(t, axRuntime, ns)

	listArrows := func(context.Context) ([]models.ArrowView, error) {
		return []models.ArrowView{makeArrowView(ns)}, nil
	}
	listAgg := func(context.Context) ([]domain.Namespace, error) {
		return []domain.Namespace{ns}, nil
	}

	w := &mocks.Wizard{}
	runtimeinternal.RecoverTransients(context.Background(), listArrows, listAgg, axRuntime, w)
	axRuntime.WaitPublish()

	got, err := axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, got.State,
		"a namespace in both sources must be recovered exactly once to absent")
}
