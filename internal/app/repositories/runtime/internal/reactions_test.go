package runtimeinternal_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	runtimeinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func newTestAsynxRuntimeForReactions(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
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

func TestRegisterReactions_Success(t *testing.T) {
	axRuntime := newTestAsynxRuntimeForReactions(t)
	w := &mocks.Wizard{}
	markInstalled := func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error {
		return nil
	}

	err := runtimeinternal.RegisterReactions(axRuntime, markInstalled, w, noopDrain())
	require.NoError(t, err)
}

func TestRegisterReactions_NilWizard_DoesNotPanic(t *testing.T) {
	axRuntime := newTestAsynxRuntimeForReactions(t)
	markInstalled := func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error {
		return nil
	}

	err := runtimeinternal.RegisterReactions(axRuntime, markInstalled, nil, noopDrain())
	require.NoError(t, err)
}

func TestOnBegun_NilExecution_NoOp(t *testing.T) {
	axRuntime := newTestAsynxRuntimeForReactions(t)
	w := &mocks.Wizard{}
	called := atomic.Bool{}
	markInstalled := func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error {
		called.Store(true)
		return nil
	}

	err := runtimeinternal.RegisterReactions(axRuntime, markInstalled, w, noopDrain())
	require.NoError(t, err)

	// Send a runtime event with nil Execution - onBegun should be a no-op
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	_, err = axRuntime.Send(context.Background(), beginExecCmdWithNilExec{ns: ns})
	require.NoError(t, err)
	axRuntime.WaitPublish()

	// markInstalled should NOT have been called since execution is nil
	assert.False(t, called.Load())
}

func TestOnBegun_NilWizard_NoOp(t *testing.T) {
	axRuntime := newTestAsynxRuntimeForReactions(t)
	called := atomic.Bool{}
	markInstalled := func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error {
		called.Store(true)
		return nil
	}

	err := runtimeinternal.RegisterReactions(axRuntime, markInstalled, nil, noopDrain())
	require.NoError(t, err)

	// Send a BeginInstall — wizard is nil so onBegun is a no-op
	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	_, err = axRuntime.Send(context.Background(), commands.BeginInstall{
		Namespace: ns,
		Steps:     domainStep.StepList{domainStep.NewRunStep("s", "echo hi", false, "", true)},
	})
	require.NoError(t, err)
	axRuntime.WaitPublish()

	// No wizard = no execution started
	assert.False(t, called.Load())
}

func TestOnBegun_WithWizard_ExecutesAndDrains(t *testing.T) {
	axRuntime := newTestAsynxRuntimeForReactions(t)
	markInstalledCalled := atomic.Bool{}
	markInstalled := func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error {
		markInstalledCalled.Store(true)
		return nil
	}

	w := &mocks.Wizard{}
	err := runtimeinternal.RegisterReactions(axRuntime, markInstalled, w, noopDrain())
	require.NoError(t, err)

	ns := domain.Namespace("github.com/user/repo@v1.0.0")
	_, err = axRuntime.Send(context.Background(), commands.BeginInstall{
		Namespace: ns,
		Steps:     domainStep.StepList{domainStep.NewRunStep("s", "echo hi", false, "", true)},
	})
	require.NoError(t, err)
	axRuntime.WaitPublish()

	// The mock wizard returns success, so MarkInstalled should be called.
	// drainExecution runs async, so poll for the result.
	deadline := time.Now().Add(500 * time.Millisecond)
	for !markInstalledCalled.Load() && time.Now().Before(deadline) {
	}
	assert.True(t, markInstalledCalled.Load())
}

func TestRegisterReactions_ShutdownAsynx_Error(t *testing.T) {
	axRuntime := newTestAsynxRuntimeForReactions(t)
	// Shut down first so Subscribe will fail.
	_ = axRuntime.Shutdown(context.Background())

	markInstalled := func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error {
		return nil
	}

	err := runtimeinternal.RegisterReactions(axRuntime, markInstalled, nil, noopDrain())
	require.Error(t, err)
}

// noopDrain returns a tryAddDrain stub that always succeeds with a no-op done.
func noopDrain() func() (func(), bool) {
	return func() (func(), bool) {
		return func() {}, true
	}
}

// ─── Seeding helpers ─────────────────────────────────────────────────────────

// beginExecCmdWithNilExec forces an event where the runtime.Execution is nil
// to test the onBegun nil-guard branch. We bypass the normal BeginExecution cmd
// by directly emitting a "begun" event with no execution.
type beginExecCmdWithNilExec struct {
	ns domain.Namespace
}

func (c beginExecCmdWithNilExec) AggregateID() string  { return c.ns.String() }
func (c beginExecCmdWithNilExec) EventName() string    { return "runtime.begun." + c.ns.String() }
func (c beginExecCmdWithNilExec) ShouldSnapshot() bool { return true }
func (c beginExecCmdWithNilExec) Validate(current *domainRuntime.ArrowRuntime) error {
	return nil
}

func (c beginExecCmdWithNilExec) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:       c.ns,
		State:     domain.ArrowStateRunning,
		Execution: nil, // explicitly nil
	}
}
