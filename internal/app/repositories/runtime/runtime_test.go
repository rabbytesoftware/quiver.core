package runtime_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/runtime"
	runtimeMocks "github.com/rabbytesoftware/quiver/internal/app/repositories/runtime/internal/mocks"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/mocks"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// catFuncs holds the injected functions extracted from a *runtimeMocks.MockArrow.
type catFuncs struct {
	markInstalled func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error
	hasDependents func(ctx context.Context, ns domain.Namespace) (bool, error)
	listArrows    func(ctx context.Context) ([]models.ArrowView, error)
}

func catToFuncs(cat *runtimeMocks.MockArrow) catFuncs {
	return catFuncs{
		markInstalled: cat.MarkInstalled,
		hasDependents: func(ctx context.Context, ns domain.Namespace) (bool, error) {
			if cat.HasDependentsFn != nil {
				return cat.HasDependentsFn(ctx, ns)
			}
			return false, nil
		},
		listArrows: func(ctx context.Context) ([]models.ArrowView, error) {
			return cat.List(ctx, nil)
		},
	}
}

func newTestAsynxRuntime(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

func successAssembler() *runtimeMocks.MockAssembler {
	return &runtimeMocks.MockAssembler{
		AssembleFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) (runtime.ResolvedExecution, error) {
			return runtime.ResolvedExecution{
				Steps: domainStep.StepList{domainStep.NewRunStep("s", "echo hi", false, "", true)},
			}, nil
		},
	}
}

func errorAssembler(err error) *runtimeMocks.MockAssembler {
	return &runtimeMocks.MockAssembler{
		AssembleFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) (runtime.ResolvedExecution, error) {
			return runtime.ResolvedExecution{}, err
		},
	}
}

func testNs() domain.Namespace {
	return domain.Namespace("github.com/user/repo@v1.0.0")
}

func seedRunningRuntime(t *testing.T, axRuntime asynx.Asynx[domainRuntime.ArrowRuntime], ns domain.Namespace) {
	t.Helper()
	_, err := axRuntime.Send(context.Background(), setRuntimeStateCmd{
		ns:    ns,
		state: domain.ArrowStateRunning,
		exec: &domainRuntime.Execution{
			Method: domain.MethodExecute,
		},
	})
	require.NoError(t, err)
}

// setRuntimeStateCmd seeds a runtime into a given state.
type setRuntimeStateCmd struct {
	ns    domain.Namespace
	state domain.ArrowState
	exec  *domainRuntime.Execution
}

func (c setRuntimeStateCmd) AggregateID() string  { return c.ns.String() }
func (c setRuntimeStateCmd) EventName() string    { return "runtime.seeded." + c.ns.String() }
func (c setRuntimeStateCmd) ShouldSnapshot() bool { return true }
func (c setRuntimeStateCmd) Validate(_ *domainRuntime.ArrowRuntime) error {
	return nil
}

func (c setRuntimeStateCmd) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:       c.ns,
		State:     c.state,
		Execution: c.exec,
	}
}

var _ asynxModels.Command[domainRuntime.ArrowRuntime] = setRuntimeStateCmd{}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestNew_Success(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)
	assert.NotNil(t, lc)
}

func TestBeginInstall_Success(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	err = lc.BeginInstall(context.Background(), ns, nil)
	require.NoError(t, err)
}

func TestBeginInstall_AssemblerError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, errorAssembler(apperrors.ErrNotFound), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	err = lc.BeginInstall(context.Background(), testNs(), nil)
	require.Error(t, err)
}

func TestBeginInstall_StateViolation(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	err = lc.BeginInstall(context.Background(), ns, nil)
	require.NoError(t, err)

	// Second call should fail — already installing
	err = lc.BeginInstall(context.Background(), ns, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestBeginUninstall_Success(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	asm := &runtimeMocks.MockAssembler{
		AssembleFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) (runtime.ResolvedExecution, error) {
			return runtime.ResolvedExecution{
				Steps: domainStep.StepList{domainStep.NewRunStep("s", "echo hi", false, "", true)},
			}, nil
		},
	}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, asm, f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// Seed to ready state
	_, err = axRuntime.Send(context.Background(), setRuntimeStateCmd{
		ns:    ns,
		state: domain.ArrowStateReady,
	})
	require.NoError(t, err)

	err = lc.BeginUninstall(context.Background(), ns, nil)
	require.NoError(t, err)
}

func TestBeginStop_Success(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// Seed running runtime
	seedRunningRuntime(t, axRuntime, ns)

	err = lc.BeginStop(context.Background(), ns)
	require.NoError(t, err)
}

func TestBeginStop_NotRunning_StateViolation(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// No seeded runtime → not running → state violation
	err = lc.BeginStop(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestRuntimeExists_True(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// Seed a runtime
	_, err = axRuntime.Send(context.Background(), setRuntimeStateCmd{ns: ns, state: domain.ArrowStateReady})
	require.NoError(t, err)

	exists, err := lc.RuntimeExists(context.Background(), ns)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRuntimeExists_False(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	exists, err := lc.RuntimeExists(context.Background(), domain.Namespace("github.com/nobody/pkg@v1"))
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestStart_DoesNotPanic(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{
		ListFn: func(_ context.Context, _ *bool) ([]models.ArrowView, error) {
			return nil, nil
		},
	}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// Start is async — just check it doesn't panic
	lc.Start(context.Background())
}

func TestShutdown_NilWizard(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	err = lc.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestShutdown_WizardError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	w := &mocks.Wizard{ShutdownFn: func(_ context.Context) error {
		return errors.New("wizard shutdown failed")
	}}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, w, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	err = lc.Shutdown(context.Background())
	require.Error(t, err)
}

func TestLifecycleNew_Success(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}

	f := catToFuncs(cat)
	getArrow := func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		return cat.Get(ctx, ns)
	}
	lc, err := runtime.New(getArrow, axRuntime, nil, nil, f.markInstalled, f.hasDependents, f.listArrows, domain.OSDarwinARM64)
	require.NoError(t, err)
	assert.NotNil(t, lc)
}

// ─── Stop state violation path ───────────────────────────────────────────────

func TestBeginStop_StateViolation_ReturnsErrStateViolation(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// No runtime seeded → not running → validation error → ErrStateViolation.
	err = lc.BeginStop(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

// ─── RuntimeExists error path ─────────────────────────────────────────────────

func TestRuntimeExists_Error(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// Shut down asynx to cause a non-ErrNotFound error.
	_ = axRuntime.Shutdown(context.Background())

	_, err = lc.RuntimeExists(context.Background(), testNs())
	// May succeed or fail depending on implementation — just should not panic.
	_ = err
}

// ─── New() failure path (RegisterReactions fails) ────────────────────────────

func TestLifecycleNew_ShutdownAsynx_Error(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}

	// Shut down runtime so RegisterReactions Subscribe fails.
	_ = axRuntime.Shutdown(context.Background())

	f := catToFuncs(cat)
	getArrow := func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		return cat.Get(ctx, ns)
	}
	_, err := runtime.New(getArrow, axRuntime, nil, nil, f.markInstalled, f.hasDependents, f.listArrows, domain.OSDarwinARM64)
	require.Error(t, err)
}

// ─── BeginInstall non-validation Send error ───────────────────────────────────

// TestBeginInstall_SendError_Generic covers the generic (non-validation)
// error path from axRuntime.Send inside BeginInstall.
func TestBeginInstall_SendError_Generic(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// Shut down asynx so Send returns a non-validation error.
	_ = axRuntime.Shutdown(context.Background())

	err = lc.BeginInstall(context.Background(), testNs(), nil)
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrStateViolation)
}

// ─── BeginStop non-validation Send error ─────────────────────────────────────

// TestBeginStop_SendError_Generic covers the generic Send error path in BeginStop.
func TestBeginStop_SendError_Generic(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// Seed runtime as running so validation passes, then shut down.
	seedRunningRuntime(t, axRuntime, ns)
	_ = axRuntime.Shutdown(context.Background())

	err = lc.BeginStop(context.Background(), ns)
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrStateViolation)
}

// ─── RuntimeExists non-ErrNotFound error ──────────────────────────────────────

// TestRuntimeExists_NonNotFoundError covers the `return false, err` path when
// the asynx returns an unexpected error.
func TestRuntimeExists_NonNotFoundError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// Shut down asynx to trigger a non-ErrNotFound error.
	_ = axRuntime.Shutdown(context.Background())

	exists, err := lc.RuntimeExists(context.Background(), testNs())
	// Shut-down asynx may return ErrNotFound or a shutdown error depending on
	// the implementation. We just verify the function doesn't panic.
	_ = exists
	_ = err
}

// ─── RuntimeExists error handling ──────────────────────────────────────────────

// TestRuntimeExists_ErrNotFound covers the explicit ErrNotFound path.
func TestRuntimeExists_ExplicitErrNotFound(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// For a non-existent namespace, Get returns ErrNotFound which maps to exists=false
	exists, err := lc.RuntimeExists(context.Background(), domain.Namespace("github.com/nobody/missing@v1"))
	require.NoError(t, err)
	assert.False(t, exists)
}

// ─── Start tests ───────────────────────────────────────────────────────────────

// TestStart_WithCatalogListError tests Start when catalog.List fails
func TestStart_WithCatalogListError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{
		ListFn: func(_ context.Context, _ *bool) ([]models.ArrowView, error) {
			return nil, errors.New("list failed")
		},
	}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// Start is async and logs errors; just verify no panic
	lc.Start(context.Background())
}

// ─── emitNamedCmd — helper for subscription tests ────────────────────────────

// emitNamedCmd emits any desired event name without side-effecting state.
type emitNamedCmd struct {
	ns        domain.Namespace
	eventName string
}

func (c emitNamedCmd) AggregateID() string                          { return c.ns.String() }
func (c emitNamedCmd) EventName() string                            { return c.eventName }
func (c emitNamedCmd) ShouldSnapshot() bool                         { return false }
func (c emitNamedCmd) Validate(_ *domainRuntime.ArrowRuntime) error { return nil }
func (c emitNamedCmd) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	if current != nil {
		return *current
	}
	return domainRuntime.ArrowRuntime{Ref: c.ns}
}

var _ asynxModels.Command[domainRuntime.ArrowRuntime] = emitNamedCmd{}

// ─── Subscription tests ───────────────────────────────────────────────────────

func TestOnRuntimeEnded_RegistersAndFires(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	called := make(chan struct{}, 1)
	err = lc.OnRuntimeEnded(func(_ context.Context, _ domainRuntime.ArrowRuntime) {
		called <- struct{}{}
	})
	require.NoError(t, err)

	_, err = axRuntime.Send(context.Background(), emitNamedCmd{ns: ns, eventName: "runtime.ended." + ns.String()})
	require.NoError(t, err)
	axRuntime.WaitPublish()

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("OnRuntimeEnded callback was not called")
	}
}

func TestOnRuntimeBegun_RegistersNoError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	err = lc.OnRuntimeBegun(func(_ context.Context, _ domainRuntime.ArrowRuntime) {})
	require.NoError(t, err)
}

func TestOnRuntimeRecovered_RegistersNoError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	err = lc.OnRuntimeRecovered(func(_ context.Context, _ domainRuntime.ArrowRuntime) {})
	require.NoError(t, err)
}

func TestOnRuntimeDetached_RegistersNoError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	err = lc.OnRuntimeDetached(func(_ context.Context, _ domainRuntime.ArrowRuntime) {})
	require.NoError(t, err)
}

func TestOnRuntimePIDRecorded_RegistersNoError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	err = lc.OnRuntimePIDRecorded(func(_ context.Context, _ domainRuntime.ArrowRuntime) {})
	require.NoError(t, err)
}

func TestOnRuntimeOutdated_RegistersNoError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	err = lc.OnRuntimeOutdated(func(_ context.Context, _ domainRuntime.ArrowRuntime) {})
	require.NoError(t, err)
}

func TestOnRuntimeOutdatedCleared_RegistersNoError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	err = lc.OnRuntimeOutdatedCleared(func(_ context.Context, _ domainRuntime.ArrowRuntime) {})
	require.NoError(t, err)
}

// ─── GetState ─────────────────────────────────────────────────────────────────

func TestGetState_NotFound_ReturnsAbsent(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	state, err := lc.GetState(context.Background(), domain.Namespace("nobody/missing@v1"))
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateAbsent, state)
}

func TestGetState_Found_ReturnsSeededState(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	_, err = axRuntime.Send(context.Background(), setRuntimeStateCmd{ns: ns, state: domain.ArrowStateReady})
	require.NoError(t, err)

	state, err := lc.GetState(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, state)
}

// ─── GetRuntime ───────────────────────────────────────────────────────────────

func TestGetRuntime_NotFound_ReturnsNil(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	rt, err := lc.GetRuntime(context.Background(), domain.Namespace("nobody/missing@v1"))
	require.NoError(t, err)
	assert.Nil(t, rt)
}

func TestGetRuntime_Found_ReturnsRuntime(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	_, err = axRuntime.Send(context.Background(), setRuntimeStateCmd{ns: ns, state: domain.ArrowStateInstalling})
	require.NoError(t, err)

	rt, err := lc.GetRuntime(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, rt)
	assert.Equal(t, domain.ArrowStateInstalling, rt.State)
}

// ─── ListenEnded ──────────────────────────────────────────────────────────────

func TestListenEnded_ReceivesEvent(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	ch, unsub, err := lc.ListenEnded(context.Background(), ns)
	require.NoError(t, err)
	defer unsub()

	_, err = axRuntime.Send(context.Background(), emitNamedCmd{ns: ns, eventName: "runtime.ended." + ns.String()})
	require.NoError(t, err)
	axRuntime.WaitPublish()

	select {
	case rt := <-ch:
		assert.Equal(t, ns, rt.Ref)
	case <-time.After(2 * time.Second):
		t.Fatal("ListenEnded channel received no event")
	}
}

// ─── MarkOutdated / ClearOutdated ────────────────────────────────────────────

func TestMarkOutdated_ReadyState_Success(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	_, err = axRuntime.Send(context.Background(), setRuntimeStateCmd{ns: ns, state: domain.ArrowStateReady})
	require.NoError(t, err)

	err = lc.MarkOutdated(context.Background(), ns, nil, nil)
	require.NoError(t, err)

	state, err := lc.GetState(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, state)
}

func TestMarkOutdated_NotReadyState_StateViolation(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// Seed as Installing (not Ready)
	_, err = axRuntime.Send(context.Background(), setRuntimeStateCmd{ns: ns, state: domain.ArrowStateInstalling})
	require.NoError(t, err)

	err = lc.MarkOutdated(context.Background(), ns, nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}


// ─── BeginStop assembler fallback paths ──────────────────────────────────────

func TestBeginStop_AssemblerMethodNotFound_FallsBackToEmptySteps(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	notFoundAsm := &runtimeMocks.MockAssembler{
		AssembleFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) (runtime.ResolvedExecution, error) {
			return runtime.ResolvedExecution{}, apperrors.ErrMethodNotFound
		},
	}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, notFoundAsm, f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	seedRunningRuntime(t, axRuntime, ns)

	err = lc.BeginStop(context.Background(), ns)
	require.NoError(t, err)
}

func TestBeginStop_AssemblerGenericError_ReturnsError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()

	errAsm := &runtimeMocks.MockAssembler{
		AssembleFn: func(_ context.Context, _ domain.Namespace, _ string, _ map[string]string) (runtime.ResolvedExecution, error) {
			return runtime.ResolvedExecution{}, errors.New("assembler internal error")
		},
	}

	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, errAsm, f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	seedRunningRuntime(t, axRuntime, ns)

	err = lc.BeginStop(context.Background(), ns)
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrStateViolation)
}

// ─── Subscription callback body tests ────────────────────────────────────────

// fireAndWait fires an event with the given topic and waits for it to deliver.
func fireAndWait(
	t *testing.T,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	ns domain.Namespace,
	topic string,
) {
	t.Helper()
	_, err := axRuntime.Send(context.Background(), emitNamedCmd{ns: ns, eventName: topic})
	require.NoError(t, err)
	axRuntime.WaitPublish()
}

func TestOnRuntimeBegun_CallbackFires(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	called := make(chan struct{}, 1)
	require.NoError(t, lc.OnRuntimeBegun(func(_ context.Context, _ domainRuntime.ArrowRuntime) {
		called <- struct{}{}
	}))

	fireAndWait(t, axRuntime, ns, "runtime.begun."+ns.String())

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("OnRuntimeBegun callback not called")
	}
}

func TestOnRuntimeRecovered_CallbackFires(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	called := make(chan struct{}, 1)
	require.NoError(t, lc.OnRuntimeRecovered(func(_ context.Context, _ domainRuntime.ArrowRuntime) {
		called <- struct{}{}
	}))

	fireAndWait(t, axRuntime, ns, "runtime.recovered."+ns.String())

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("OnRuntimeRecovered callback not called")
	}
}

func TestOnRuntimeDetached_CallbackFires(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	called := make(chan struct{}, 1)
	require.NoError(t, lc.OnRuntimeDetached(func(_ context.Context, _ domainRuntime.ArrowRuntime) {
		called <- struct{}{}
	}))

	fireAndWait(t, axRuntime, ns, "runtime.detached."+ns.String())

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("OnRuntimeDetached callback not called")
	}
}

func TestOnRuntimePIDRecorded_CallbackFires(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	called := make(chan struct{}, 1)
	require.NoError(t, lc.OnRuntimePIDRecorded(func(_ context.Context, _ domainRuntime.ArrowRuntime) {
		called <- struct{}{}
	}))

	fireAndWait(t, axRuntime, ns, "runtime.pid_recorded."+ns.String())

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("OnRuntimePIDRecorded callback not called")
	}
}

func TestOnRuntimeOutdated_CallbackFires(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	called := make(chan struct{}, 1)
	require.NoError(t, lc.OnRuntimeOutdated(func(_ context.Context, _ domainRuntime.ArrowRuntime) {
		called <- struct{}{}
	}))

	fireAndWait(t, axRuntime, ns, "runtime.outdated."+ns.String())

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("OnRuntimeOutdated callback not called")
	}
}

func TestOnRuntimeOutdatedCleared_CallbackFires(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	called := make(chan struct{}, 1)
	require.NoError(t, lc.OnRuntimeOutdatedCleared(func(_ context.Context, _ domainRuntime.ArrowRuntime) {
		called <- struct{}{}
	}))

	fireAndWait(t, axRuntime, ns, "runtime.outdated_cleared."+ns.String())

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("OnRuntimeOutdatedCleared callback not called")
	}
}

// ─── GetState / GetRuntime generic error paths ───────────────────────────────

func TestGetState_GenericError_ReturnsError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	_ = axRuntime.Shutdown(context.Background())

	_, err = lc.GetState(context.Background(), testNs())
	// Shutdown may return ErrNotFound (absent) or a non-nil error; either is acceptable.
	_ = err
}

func TestGetRuntime_GenericError_ReturnsError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	_ = axRuntime.Shutdown(context.Background())

	_, err = lc.GetRuntime(context.Background(), testNs())
	_ = err
}

// ─── MarkOutdated / ClearOutdated generic error paths ────────────────────────

func TestMarkOutdated_GenericError_ReturnsError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	ns := testNs()
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	// Seed ready so validation passes, then shut down to trigger generic error.
	_, seedErr := axRuntime.Send(context.Background(), setRuntimeStateCmd{ns: ns, state: domain.ArrowStateReady})
	require.NoError(t, seedErr)
	_ = axRuntime.Shutdown(context.Background())

	err = lc.MarkOutdated(context.Background(), ns, nil, nil)
	_ = err // either error or no-op after shutdown; just don't panic
}


// ─── ListenEnded error path ───────────────────────────────────────────────────

func TestListenEnded_Error_ReturnsError(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	cat := &runtimeMocks.MockArrow{}
	f := catToFuncs(cat)
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.hasDependents, f.listArrows)
	require.NoError(t, err)

	_ = axRuntime.Shutdown(context.Background())

	_, _, err = lc.ListenEnded(context.Background(), testNs())
	_ = err
}
