package wizard

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestWizard(t *testing.T) *wizard {
	t.Helper()
	w, err := New()
	require.NoError(t, err)
	return w.(*wizard)
}

func newTestReq(steps ...domainstep.Step) RunRequest {
	return RunRequest{
		Namespace: domain.Namespace("test/user/repo/arrow"),
		Variables: map[string]string{},
		Steps:     steps,
		WorkDir:   os.TempDir(),
	}
}

func TestNew(t *testing.T) {
	w, err := New()
	require.NoError(t, err)
	require.NotNil(t, w)
	assert.Implements(t, (*Wizard)(nil), w)
}

func TestExecute_EmptySteps(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}

	err := w.Execute(context.Background(), newTestReq(), rep)

	require.NoError(t, err)
	assert.Empty(t, rep.Started)
	assert.Empty(t, rep.Completed)
	assert.Empty(t, rep.Failed)
}

func TestExecute_StepTypeMismatch_NoPanic(t *testing.T) {
	// A Step whose Type() matches a registered handler but whose concrete type
	// does not — e.g. a mock reporting StepTypeRun while not being a RunStep.
	// adapt must return an error, never panic.
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	s := mocks.Step{TypeVal: domainstep.StepTypeRun, ExitOnFailureVal: true}

	assert.NotPanics(t, func() {
		err := w.Execute(context.Background(), newTestReq(s), rep)
		var stepErr *StepError
		require.True(t, errors.As(err, &stepErr))
	})
}

func TestExecute_UnknownStepType_Continue(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}

	err := w.Execute(context.Background(), newTestReq(mocks.Step{TypeVal: "unknown"}), rep)

	require.NoError(t, err, "unknown step with ExitOnFailure=false should not propagate error")
	assert.Equal(t, []int{0}, rep.Failed)
}

func TestExecute_UnknownStepType_ExitOnFailure(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	s := mocks.Step{TypeVal: "unknown", ExitOnFailureVal: true}

	err := w.Execute(context.Background(), newTestReq(s), rep)

	var stepErr *StepError
	require.True(t, errors.As(err, &stepErr))
	assert.True(t, errors.Is(stepErr.Cause, ErrUnknownStepType))
}

func TestExecute_SingleRunStep(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	s := domainstep.NewRunStep("echo", "echo hello", 5*time.Second, true)

	err := w.Execute(context.Background(), newTestReq(s), rep)

	require.NoError(t, err)
	assert.Equal(t, []int{0}, rep.Started)
	assert.Equal(t, []int{0}, rep.Completed)
	assert.Empty(t, rep.Failed)
}

func TestExecute_StepFailure_ExitOnFailure(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	s := domainstep.NewRunStep("fail", "false", 5*time.Second, true)

	err := w.Execute(context.Background(), newTestReq(s), rep)

	var stepErr *StepError
	require.True(t, errors.As(err, &stepErr))
	assert.Equal(t, 0, stepErr.Index)
	assert.Equal(t, []int{0}, rep.Started)
	assert.Equal(t, []int{0}, rep.Failed)
}

func TestExecute_StepFailure_ContinueOnFailure(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	fail := domainstep.NewRunStep("fail", "false", 5*time.Second, false)
	ok := domainstep.NewRunStep("ok", "echo done", 5*time.Second, true)

	err := w.Execute(context.Background(), newTestReq(fail, ok), rep)

	require.NoError(t, err)
	assert.Equal(t, []int{0, 1}, rep.Started)
	assert.Equal(t, []int{0}, rep.Failed)
	assert.Equal(t, []int{1}, rep.Completed)
}

func TestExecute_ConcurrentSameNamespace(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	long := domainstep.NewRunStep("sleep", "sleep 10", 30*time.Second, true)
	req := newTestReq(long)

	started := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		close(started)
		done <- w.Execute(context.Background(), req, rep)
	}()

	<-started

	err := w.Execute(context.Background(), req, &mocks.Reporter{})
	assert.ErrorIs(t, err, ErrExecutionExists)

	w.Cancel(req.Namespace)
	<-done
}

func TestExecute_CleansUpFinishedProcesses(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	s := domainstep.NewRunStep("echo", "echo hello", 5*time.Second, true)

	err := w.Execute(context.Background(), newTestReq(s), rep)
	require.NoError(t, err)

	assert.Equal(t, 0, w.runtime.Count(), "finished processes should be cleaned up after Execute")
}

func TestCancel_RunningExecution(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	long := domainstep.NewRunStep("sleep", "sleep 10", 30*time.Second, true)
	req := newTestReq(long)

	done := make(chan error, 1)
	started := make(chan struct{})

	go func() {
		close(started)
		done <- w.Execute(context.Background(), req, rep)
	}()

	<-started
	w.Cancel(req.Namespace)

	err := <-done
	assert.Error(t, err, "cancelled execution should return an error")
}

func TestCancel_NoExecution(t *testing.T) {
	w := newTestWizard(t)
	assert.NotPanics(t, func() {
		w.Cancel(domain.Namespace("test/user/repo/none"))
	})
}

func TestCancel_WrongTypeInExecutions(t *testing.T) {
	w := newTestWizard(t)
	ns := domain.Namespace("test/user/repo/arrow")
	w.executions.Store(ns.String(), "not-an-execution-state")
	assert.NotPanics(t, func() {
		w.Cancel(ns)
	})
}

func TestCancel_GracefulEscalation(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	long := domainstep.NewRunStep("sleep", "sleep 100", 30*time.Second, true)
	req := newTestReq(long)

	done := make(chan error, 1)
	started := make(chan struct{})

	go func() {
		close(started)
		done <- w.Execute(context.Background(), req, rep)
	}()

	<-started
	w.Cancel(req.Namespace)

	err := <-done
	assert.Error(t, err, "cancelled execution should return an error")
}
