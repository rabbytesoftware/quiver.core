package wizard

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/mocks"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
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
		case EventKindEnded:
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

func TestWizard_Shutdown_DoesNotCancelExecuteMethodExecution(t *testing.T) {
	w, err := New(nil)
	require.NoError(t, err)

	long := domainstep.NewRunStep("sleep", "sleep 2", false, "30s", true)
	req := RunRequest{
		Namespace: "test/user/repo/arrow",
		Method:    domain.MethodExecute,
		Variables: map[string]string{},
		Steps:     []domainstep.Step{long},
		WorkDir:   os.TempDir(),
	}
	exec := w.Start(context.Background(), req)

	// Wait for the process to actually start, then shut the wizard down.
	// _execute survives shutdown, so Shutdown returns immediately without
	// waiting for this execution — the loop keeps draining below (rather
	// than breaking here) so the assertion below only runs once Finish has
	// actually closed the events channel, regardless of how fast Shutdown
	// itself returns.
	for ev := range exec.Events() {
		if ev.Kind == EventKindPID {
			require.NoError(t, w.Shutdown(context.Background()))
		}
	}

	// The sleep must complete naturally (outcome Success), not be cancelled
	// by the Shutdown call above — proving it genuinely outlived the wizard's
	// own shutdown signal rather than merely racing it.
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, exec.Outcome())
}

func TestWizard_Shutdown_DoesNotCancelCustomMethodExecution(t *testing.T) {
	w, err := New(nil)
	require.NoError(t, err)

	long := domainstep.NewRunStep("sleep", "sleep 2", false, "30s", true)
	req := RunRequest{
		Namespace: "test/user/repo/arrow",
		Method:    "start", // a manifest-defined custom method (see methods.start), not one of the four one-shot lifecycle methods
		Variables: map[string]string{},
		Steps:     []domainstep.Step{long},
		WorkDir:   os.TempDir(),
	}
	exec := w.Start(context.Background(), req)

	// Mirrors TestWizard_Shutdown_DoesNotCancelExecuteMethodExecution: the
	// cancel-on-shutdown set is a finite whitelist of the four one-shot
	// lifecycle methods, not everything-but-_execute — a custom method (as
	// used by the service-running integration fixture's methods.start) must
	// survive shutdown exactly like _execute does.
	for ev := range exec.Events() {
		if ev.Kind == EventKindPID {
			require.NoError(t, w.Shutdown(context.Background()))
		}
	}

	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, exec.Outcome())
}

func TestWizard_Shutdown_DoesNotWaitForSurvivingExecution(t *testing.T) {
	w, err := New(nil)
	require.NoError(t, err)

	long := domainstep.NewRunStep("sleep", "sleep 2", false, "30s", true)
	req := RunRequest{
		Namespace: "test/user/repo/arrow",
		Method:    domain.MethodExecute,
		Variables: map[string]string{},
		Steps:     []domainstep.Step{long},
		WorkDir:   os.TempDir(),
	}
	exec := w.Start(context.Background(), req)

	for ev := range exec.Events() {
		if ev.Kind == EventKindPID {
			break
		}
	}

	// A surviving _execute must not be waited on: bound Shutdown far below
	// the still-running sleep 2 and require it to return nil, not
	// DeadlineExceeded. Before splitting w.wg to exclude survivors, Shutdown
	// blocked on every active execution regardless of method, so this would
	// have timed out here.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	require.NoError(t, w.Shutdown(ctx))
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

// ─── Probe ───────────────────────────────────────────────────────────────────

func TestProbe_NoSteps_Detects(t *testing.T) {
	w := newTestWizard(t)

	assert.NoError(t, w.Probe(context.Background(), newTestReq()))
}

func TestProbe_AllStepsSucceed_Detects(t *testing.T) {
	w := newTestWizard(t)
	req := newTestReq(
		domainstep.NewRunStep("first", "true", false, "5s", true),
		domainstep.NewRunStep("second", "echo found", false, "5s", true),
	)

	assert.NoError(t, w.Probe(context.Background(), req))
}

// TestProbe_FailingStep_DoesNotDetect: a probe answers a question, so
// exit_on_failure has no say — the first failure is the answer, and no later
// step runs to contradict it.
func TestProbe_FailingStep_DoesNotDetect(t *testing.T) {
	w := newTestWizard(t)
	marker := filepath.Join(t.TempDir(), "ran")
	req := newTestReq(
		domainstep.NewRunStep("missing", "false", false, "5s", false),
		domainstep.NewRunStep("after", "touch "+marker, false, "5s", false),
	)

	err := w.Probe(context.Background(), req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "probe step 0")
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "no step may run after the answer is known")
}

func TestProbe_ExpandsVariables(t *testing.T) {
	w := newTestWizard(t)
	marker := filepath.Join(t.TempDir(), "marker")
	require.NoError(t, os.WriteFile(marker, []byte("x"), 0o600))

	req := newTestReq(domainstep.NewRunStep("detect", "test -f ${MARKER}", false, "5s", true))
	req.Variables = map[string]string{"MARKER": marker}

	assert.NoError(t, w.Probe(context.Background(), req))
}

func TestProbe_UnknownStepType_DoesNotDetect(t *testing.T) {
	w := newTestWizard(t)

	err := w.Probe(context.Background(), newTestReq(mocks.Step{TypeVal: "unknown"}))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownStepType)
}

func TestProbe_CancelledContext_DoesNotDetect(t *testing.T) {
	w := newTestWizard(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := w.Probe(ctx, newTestReq(domainstep.NewRunStep("echo", "echo hi", false, "5s", true)))

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestProbe_AfterShutdown_Refused is why Probe exists rather than a call to
// Start with a made-up method name: IsOneShotMethod would classify a probe as a
// supervised process, and the wizard would neither refuse it here nor cancel it
// once it had begun.
func TestProbe_AfterShutdown_Refused(t *testing.T) {
	w := newTestWizard(t)
	require.NoError(t, w.Shutdown(context.Background()))

	err := w.Probe(context.Background(), newTestReq())

	require.ErrorIs(t, err, ErrShuttingDown)
}

// TestProbe_ShutdownCancelsInFlight: a probe already running when Shutdown
// starts is cancelled and waited for, exactly as a one-shot lifecycle method
// would be.
func TestProbe_ShutdownCancelsInFlight(t *testing.T) {
	w := newTestWizard(t)
	errs := make(chan error, 1)
	go func() {
		errs <- w.Probe(context.Background(),
			newTestReq(domainstep.NewRunStep("sleep", "sleep 30", false, "60s", true)))
	}()

	// Give the step time to actually spawn before shutting down, so this
	// exercises cancellation rather than the refusal path above.
	time.Sleep(200 * time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, w.Shutdown(shutdownCtx), "Shutdown must wait for the in-flight probe")

	select {
	case err := <-errs:
		require.Error(t, err, "a cancelled probe never detects")
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not return after shutdown")
	}
}

// TestProbe_UnboundedStep_CutOffByCeiling proves maxProbeDuration is a real
// ceiling, not just a comment. A step declaring a timeout longer than the
// ceiling, on a caller context with no deadline of its own (Add's own
// request context is exactly this shape), must still be cut off at
// maxProbeDuration — not run for as long as the step's own declared timeout
// or the caller's context would otherwise allow. A probe runs synchronously
// on Add's own request goroutine, so without this ceiling a slow or
// unbounded manifest-supplied step would hold that request open
// indefinitely.
func TestProbe_UnboundedStep_CutOffByCeiling(t *testing.T) {
	w := newTestWizard(t)
	start := time.Now()

	err := w.Probe(context.Background(),
		newTestReq(domainstep.NewRunStep("sleep", "sleep 90", false, "5m", true)))
	elapsed := time.Since(start)

	require.Error(t, err, "a step that outlives the ceiling must not report a detection")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, maxProbeDuration+10*time.Second,
		"the ceiling must cut the probe off near maxProbeDuration, not let it run for anywhere near its own 5m declared timeout or the 90s sleep")
	assert.GreaterOrEqual(t, elapsed, maxProbeDuration-time.Second,
		"the probe must not return suspiciously early either — it should run right up to the ceiling")
}
