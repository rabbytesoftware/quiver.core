package wizard

import (
	"context"
	"os"
	"testing"

	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/mocks"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRecord holds events collected from an Execution for assertions.
type testRecord struct {
	Started   []int
	Completed []int
	Failed    []int
	PIDs      []int
	Outcome   domainRuntime.ExecutionOutcome
}

func collectEvents(
	exec Execution,
) testRecord {
	var rec testRecord
	for e := range exec.Events() {
		switch e.Kind {
		case EventKindStepStarted:
			rec.Started = append(rec.Started, e.StepIndex)
		case EventKindStepCompleted:
			rec.Completed = append(rec.Completed, e.StepIndex)
		case EventKindStepFailed:
			rec.Failed = append(rec.Failed, e.StepIndex)
		case EventKindPID:
			rec.PIDs = append(rec.PIDs, e.PID)
		}
	}
	rec.Outcome = exec.Outcome()
	return rec
}

func runSync(
	ctx context.Context,
	w Wizard,
	req RunRequest,
) testRecord {
	return collectEvents(w.Start(ctx, req))
}

func newTestWizard(t *testing.T) Wizard {
	t.Helper()
	w, err := New(nil)
	require.NoError(t, err)
	return w
}

func newTestReq(steps ...domainstep.Step) RunRequest {
	return RunRequest{
		Namespace: "test/user/repo/arrow",
		Method:    "_install",
		Variables: map[string]string{},
		Steps:     steps,
		WorkDir:   os.TempDir(),
	}
}

func TestNew(t *testing.T) {
	w, err := New(nil)
	require.NoError(t, err)
	require.NotNil(t, w)
	assert.Implements(t, (*Wizard)(nil), w)
}

func TestStart_EmptySteps(t *testing.T) {
	w := newTestWizard(t)
	rec := runSync(context.Background(), w, newTestReq())

	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, rec.Outcome)
	assert.Empty(t, rec.Started)
	assert.Empty(t, rec.Completed)
	assert.Empty(t, rec.Failed)
}

func TestStart_StepTypeMismatch_NoPanic(t *testing.T) {
	w := newTestWizard(t)
	s := mocks.Step{TypeVal: domainstep.StepTypeRun, ExitOnFailureVal: true}

	assert.NotPanics(t, func() {
		rec := runSync(context.Background(), w, newTestReq(s))
		assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, rec.Outcome)
	})
}

func TestStart_UnknownStepType_Continue(t *testing.T) {
	w := newTestWizard(t)
	rec := runSync(context.Background(), w, newTestReq(mocks.Step{TypeVal: "unknown"}))

	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, rec.Outcome)
	assert.Equal(t, []int{0}, rec.Failed)
}

func TestStart_UnknownStepType_ExitOnFailure(t *testing.T) {
	w := newTestWizard(t)
	s := mocks.Step{TypeVal: "unknown", ExitOnFailureVal: true}

	rec := runSync(context.Background(), w, newTestReq(s))

	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, rec.Outcome)
	assert.Equal(t, []int{0}, rec.Failed)
}

func TestStart_SingleRunStep(t *testing.T) {
	w := newTestWizard(t)
	s := domainstep.NewRunStep("echo", "echo hello", false, "5s", true)

	rec := runSync(context.Background(), w, newTestReq(s))

	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, rec.Outcome)
	assert.Equal(t, []int{0}, rec.Started)
	assert.Equal(t, []int{0}, rec.Completed)
	assert.Empty(t, rec.Failed)
}

func TestStart_StepFailure_ExitOnFailure(t *testing.T) {
	w := newTestWizard(t)
	s := domainstep.NewRunStep("fail", "false", false, "5s", true)

	rec := runSync(context.Background(), w, newTestReq(s))

	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, rec.Outcome)
	assert.Equal(t, []int{0}, rec.Started)
	assert.Equal(t, []int{0}, rec.Failed)
}

func TestStart_StepFailure_ContinueOnFailure(t *testing.T) {
	w := newTestWizard(t)
	fail := domainstep.NewRunStep("fail", "false", false, "5s", false)
	ok := domainstep.NewRunStep("ok", "echo done", false, "5s", true)

	rec := runSync(context.Background(), w, newTestReq(fail, ok))

	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, rec.Outcome)
	assert.Equal(t, []int{0, 1}, rec.Started)
	assert.Equal(t, []int{0}, rec.Failed)
	assert.Equal(t, []int{1}, rec.Completed)
}

func TestStart_ContextCancelled_StepsAbort(t *testing.T) {
	w := newTestWizard(t)
	long := domainstep.NewRunStep("sleep", "sleep 10", false, "30s", true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rec := runSync(ctx, w, newTestReq(long))
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, rec.Outcome)
}

func TestStart_CtxCancelledBetweenSteps_StopsEarly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	executed := 0
	w, err := New(func(c context.Context, _ wizstep.Request, _ domainstep.DependenciesStep) error {
		executed++
		cancel()
		return nil
	})
	require.NoError(t, err)

	s1 := domainstep.NewDependenciesStep("a")
	s2 := domainstep.NewDependenciesStep("b")

	rec := runSync(ctx, w, newTestReq(s1, s2))

	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, rec.Outcome)
	assert.Equal(t, 1, executed, "only first step should have executed")
}

func TestStart_RunStep_EmitsPIDEvent(t *testing.T) {
	w := newTestWizard(t)
	s := domainstep.NewRunStep("echo", "echo hello", false, "5s", true)

	rec := runSync(context.Background(), w, newTestReq(s))

	require.Len(t, rec.PIDs, 1, "should emit exactly one PID event")
	assert.Greater(t, rec.PIDs[0], 0)
}

func TestWizard_Shutdown_Empty(t *testing.T) {
	w, err := New(nil)
	require.NoError(t, err)

	err = w.Shutdown(context.Background())
	require.NoError(t, err)
}

func TestWizard_Shutdown_CancelledContext(t *testing.T) {
	w, err := New(nil)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = w.Shutdown(ctx)
}

func TestWizard_Shutdown_CancelsActiveExecution(t *testing.T) {
	w, err := New(nil)
	require.NoError(t, err)

	long := domainstep.NewRunStep("sleep", "sleep 10", false, "30s", true)
	exec := w.Start(context.Background(), newTestReq(long))

	// Consume events until the PID fires (process has started), then shut down.
	// Remaining events drain naturally as the execution is cancelled.
	for ev := range exec.Events() {
		if ev.Kind == EventKindPID {
			go w.Shutdown(context.Background())
		}
	}

	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, exec.Outcome())
}

func TestStart_CtxCancelledDuringLastStep_ReturnsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	w, err := New(func(_ context.Context, _ wizstep.Request, _ domainstep.DependenciesStep) error {
		cancel() // step succeeds but context is cancelled mid-execution
		return nil
	})
	require.NoError(t, err)

	rec := runSync(ctx, w, newTestReq(domainstep.NewDependenciesStep("last")))
	assert.Equal(t, domainRuntime.ExecutionOutcomeCancelled, rec.Outcome)
}

func TestNew_WithDepExecutor_InvokedOnDependenciesStep(t *testing.T) {
	called := false
	w, err := New(func(_ context.Context, _ wizstep.Request, _ domainstep.DependenciesStep) error {
		called = true
		return nil
	})
	require.NoError(t, err)

	dep := domainstep.NewDependenciesStep("test")
	rec := runSync(context.Background(), w, RunRequest{
		Namespace: "github.com/test/arrow",
		Method:    "_install",
		Steps:     []domainstep.Step{dep},
	})

	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, rec.Outcome)
	assert.True(t, called)
}

func TestWizard_ProcessAlive_ZeroPID(t *testing.T) {
	w, err := New(nil)
	require.NoError(t, err)

	// PID 0 is never a valid process; ProcessAlive should return false.
	if w.ProcessAlive(0) {
		t.Error("ProcessAlive(0) should return false")
	}
}

func TestWizard_New_CreatesNonNilWizard(t *testing.T) {
	w, err := New(nil)
	require.NoError(t, err)
	require.NotNil(t, w)
}
