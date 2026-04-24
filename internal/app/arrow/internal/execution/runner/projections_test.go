package runner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	arrowcmds "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectionsFixture holds all dependencies for projections tests.
type projectionsFixture struct {
	runner    *runnerService
	wiz       *mocks.Wizard
	vault     *mocks.Vault
	hookCalls []hookCall
}

type hookCall struct {
	ns      domain.Namespace
	method  string
	execErr error
	outcome domainRuntime.ExecutionOutcome
}

func newProjectionsFixture(t *testing.T) *projectionsFixture {
	t.Helper()

	wiz := &mocks.Wizard{}
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r := testRunner(t, mv)
	r.wizard = wiz

	f := &projectionsFixture{
		runner: r,
		wiz:    wiz,
		vault:  mv,
	}

	r.hook = func(
		_ context.Context,
		ns domain.Namespace,
		method string,
		execErr error,
		outcome domainRuntime.ExecutionOutcome,
	) {
		f.hookCalls = append(f.hookCalls, hookCall{
			ns:      ns,
			method:  method,
			execErr: execErr,
			outcome: outcome,
		})
	}

	return f
}

func makeProjectionsRuntime(ns domain.Namespace, method string) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:   ns,
		State: domain.ArrowStateRunning,
		Execution: &domainRuntime.Execution{
			Method:    method,
			Variables: map[string]string{},
		},
	}
}

// --- execute() ---

func TestExecute_NilActiveRun_EarlyReturn(t *testing.T) {
	f := newProjectionsFixture(t)

	rt := domainRuntime.ArrowRuntime{
		Ref:       "github.com/org/repo",
		Execution: nil,
	}

	f.runner.execute(context.Background(), rt)

	assert.False(t, f.wiz.WasExecuteCalled())
	assert.Empty(t, f.hookCalls)
}

func TestExecute_WizardSucceeds_SendsEndExecutionSuccess_CallsHook(t *testing.T) {
	f := newProjectionsFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedProjectionsRuntime(t, f.runner, ns, "_execute")

	f.wiz.ExecuteErr = nil
	rt := makeProjectionsRuntime(ns, "_execute")

	f.runner.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())

	result, err := f.runner.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, result.LastReturn.Outcome)

	require.Len(t, f.hookCalls, 1)
	assert.Equal(t, ns, f.hookCalls[0].ns)
	assert.Equal(t, "_execute", f.hookCalls[0].method)
	assert.NoError(t, f.hookCalls[0].execErr)
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, f.hookCalls[0].outcome)
}

func TestExecute_WizardCanceled_SendsEndExecutionCancelled_CallsHook(t *testing.T) {
	f := newProjectionsFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedProjectionsRuntime(t, f.runner, ns, "_execute")

	f.wiz.ExecuteErr = context.Canceled
	rt := makeProjectionsRuntime(ns, "_execute")

	f.runner.execute(context.Background(), rt)

	result, err := f.runner.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, result.LastReturn.Outcome)

	require.Len(t, f.hookCalls, 1)
	assert.ErrorIs(t, f.hookCalls[0].execErr, context.Canceled)
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, f.hookCalls[0].outcome)
}

func TestExecute_WizardFails_SendsEndExecutionFailed_CallsHook(t *testing.T) {
	f := newProjectionsFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedProjectionsRuntime(t, f.runner, ns, "_execute")

	f.wiz.ExecuteErr = errors.New("step failed")
	rt := makeProjectionsRuntime(ns, "_execute")

	f.runner.execute(context.Background(), rt)

	result, err := f.runner.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, result.LastReturn.Outcome)

	require.Len(t, f.hookCalls, 1)
	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, f.hookCalls[0].outcome)
}

func TestExecute_NilWizard_SkipsExecute_SendsEndExecution(t *testing.T) {
	f := newProjectionsFixture(t)
	f.runner.wizard = nil

	ns := domain.Namespace("github.com/org/repo")
	seedProjectionsRuntime(t, f.runner, ns, "_execute")

	rt := makeProjectionsRuntime(ns, "_execute")

	f.runner.execute(context.Background(), rt)

	assert.False(t, f.wiz.WasExecuteCalled())

	result, err := f.runner.axRuntime.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, result.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, result.LastReturn.Outcome)

	require.Len(t, f.hookCalls, 1)
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, f.hookCalls[0].outcome)
}

func TestExecute_NilHook_NoPanic(t *testing.T) {
	f := newProjectionsFixture(t)
	f.runner.hook = nil

	ns := domain.Namespace("github.com/org/repo")
	seedProjectionsRuntime(t, f.runner, ns, "_execute")

	f.wiz.ExecuteErr = nil
	rt := makeProjectionsRuntime(ns, "_execute")

	assert.NotPanics(t, func() {
		f.runner.execute(context.Background(), rt)
	})
}

func TestExecute_WithSteps_ExtractsSteps(t *testing.T) {
	f := newProjectionsFixture(t)

	ns := domain.Namespace("github.com/org/repo")
	seedProjectionsRuntime(t, f.runner, ns, "_execute")

	step := domainstep.NewRunStep("echo", "echo hello", false, "", false)
	rt := domainRuntime.ArrowRuntime{
		Ref:   ns,
		State: domain.ArrowStateRunning,
		Execution: &domainRuntime.Execution{
			Method:    "_execute",
			Variables: map[string]string{},
			Steps: []domainRuntime.StepProgress{
				{Index: 0, Status: domainRuntime.StepStatusPending, Step: step},
			},
		},
	}

	f.runner.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())
}

func TestExecute_VaultHasEntry_SetsWorkDir(t *testing.T) {
	f := newProjectionsFixture(t)
	f.vault.GetQuiverPath = "/tmp/my-arrow/quiver.json"

	ns := domain.Namespace("github.com/org/repo")
	seedProjectionsRuntime(t, f.runner, ns, "_execute")

	rt := makeProjectionsRuntime(ns, "_execute")

	f.runner.execute(context.Background(), rt)

	assert.True(t, f.wiz.WasExecuteCalled())
}

// --- handleExecution() ---

func TestHandleExecution_NilActiveRun_NoOp(t *testing.T) {
	f := newProjectionsFixture(t)

	rt := domainRuntime.ArrowRuntime{
		Ref:       "github.com/org/repo",
		Execution: nil,
	}
	evt := asynxModels.Event[domainRuntime.ArrowRuntime]{
		EventName: "runtime.begun",
		Aggregate: rt,
	}

	f.runner.handleExecution(context.Background(), evt)

	assert.False(t, f.wiz.WasExecuteCalled())
}

// --- registerProjections() ---

// failingRuntimeAsynx is a stub that fails on a specific Subscribe call.
type failingRuntimeAsynx struct {
	subscribeCallN int
	calls          int
	err            error
}

func (f *failingRuntimeAsynx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
	_ ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime],
) (string, error) {
	f.calls++
	if f.calls == f.subscribeCallN {
		return "", f.err
	}
	return "sub-id", nil
}

func (f *failingRuntimeAsynx) Send(
	_ context.Context,
	_ asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (f *failingRuntimeAsynx) SendWait(
	_ context.Context,
	_ asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (f *failingRuntimeAsynx) Shutdown(
	_ context.Context,
) error {
	return nil
}

func (f *failingRuntimeAsynx) Get(
	_ context.Context,
	_ string,
) (domainRuntime.ArrowRuntime, error) {
	return domainRuntime.ArrowRuntime{}, nil
}

func (f *failingRuntimeAsynx) Exists(
	_ context.Context,
	_ string,
) (bool, error) {
	return false, nil
}

func (f *failingRuntimeAsynx) Preload(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (f *failingRuntimeAsynx) Unsubscribe(
	_ string,
) error {
	return nil
}

func (f *failingRuntimeAsynx) Replay(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
) error {
	return nil
}

func (f *failingRuntimeAsynx) Forget(_ context.Context, _ string) error { return nil }
func (f *failingRuntimeAsynx) OnForget(_ asynxModels.ForgetHandler[domainRuntime.ArrowRuntime]) (string, error) {
	return "forget-sub-id", nil
}
func (f *failingRuntimeAsynx) WaitPublish() {
}

func TestRegisterProjections_Succeeds(t *testing.T) {
	r := testRunner(t, &mocks.Vault{GetArrowErr: vault.ErrNotCached})

	err := r.registerProjections()
	require.NoError(t, err)
}

func TestRegisterProjections_CalledByNew(t *testing.T) {
	axArrow := buildAsynxArrow(t)
	axRuntime := buildAsynxRuntime(t)
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r, err := New(axArrow, axRuntime, mv, nil, nil, domain.OSLinuxAMD64)
	require.NoError(t, err)
	assert.NotNil(t, r)
}

func TestNew_SubscribeError_ReturnsError(t *testing.T) {
	axArrow := buildAsynxArrow(t)
	subErr := assert.AnError
	failingRuntime := &failingRuntimeAsynx{subscribeCallN: 1, err: subErr}
	mv := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	_, err := New(axArrow, failingRuntime, mv, nil, nil, domain.OSLinuxAMD64)
	require.Error(t, err)
	assert.Equal(t, subErr, err)
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

// seedProjectionsRuntime sends a BeginExecution so the runtime aggregate exists.
func seedProjectionsRuntime(
	t *testing.T,
	r *runnerService,
	ns domain.Namespace,
	method string,
) {
	t.Helper()
	_, err := r.axRuntime.Send(context.Background(), arrowcmds.BeginExecution{
		Namespace: ns,
		Method:    method,
	})
	require.NoError(t, err)
	r.axRuntime.WaitPublish()
}
