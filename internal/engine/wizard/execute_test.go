package wizard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
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

func newTestReq(steps ...step.Step) ExecutionRequest {
	return ExecutionRequest{
		Namespace: domain.Namespace("test/user/repo/arrow"),
		Method:    "execute",
		Variables: map[string]string{},
		Steps:     steps,
		WorkDir:   "/tmp",
	}
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

func TestExecute_DependenciesStep_Noop(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	s := step.NewDependenciesStep("install deps")

	err := w.Execute(context.Background(), newTestReq(s), rep)

	require.NoError(t, err)
	assert.Equal(t, []int{0}, rep.Started)
	assert.Equal(t, []int{0}, rep.Completed)
	assert.Empty(t, rep.Failed)
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
	s := step.NewRunStep("echo", "echo hello", 5*time.Second, true)

	err := w.Execute(context.Background(), newTestReq(s), rep)

	require.NoError(t, err)
	assert.Equal(t, []int{0}, rep.Started)
	assert.Equal(t, []int{0}, rep.Completed)
	assert.Empty(t, rep.Failed)
}

func TestExecute_StepFailure_ExitOnFailure(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	s := step.NewRunStep("fail", "false", 5*time.Second, true)

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
	fail := step.NewRunStep("fail", "false", 5*time.Second, false)
	ok := step.NewRunStep("ok", "echo done", 5*time.Second, true)

	err := w.Execute(context.Background(), newTestReq(fail, ok), rep)

	require.NoError(t, err)
	assert.Equal(t, []int{0, 1}, rep.Started)
	assert.Equal(t, []int{0}, rep.Failed)
	assert.Equal(t, []int{1}, rep.Completed)
}

func TestExecute_ConcurrentSameNamespace(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	long := step.NewRunStep("sleep", "sleep 10", 30*time.Second, true)
	req := newTestReq(long)

	started := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		close(started)
		done <- w.Execute(context.Background(), req, rep)
	}()

	<-started
	time.Sleep(20 * time.Millisecond)

	err := w.Execute(context.Background(), req, &mocks.Reporter{})
	assert.ErrorIs(t, err, ErrExecutionExists)

	w.Cancel(req.Namespace)
	<-done
}

func TestCancel_RunningExecution(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	long := step.NewRunStep("sleep", "sleep 10", 30*time.Second, true)
	req := newTestReq(long)

	done := make(chan error, 1)
	started := make(chan struct{})

	go func() {
		close(started)
		done <- w.Execute(context.Background(), req, rep)
	}()

	<-started
	time.Sleep(20 * time.Millisecond)
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
	long := step.NewRunStep("sleep", "sleep 100", 30*time.Second, true)
	req := newTestReq(long)

	done := make(chan error, 1)
	started := make(chan struct{})

	go func() {
		close(started)
		done <- w.Execute(context.Background(), req, rep)
	}()

	<-started
	time.Sleep(50 * time.Millisecond)
	w.Cancel(req.Namespace)

	err := <-done
	assert.Error(t, err, "cancelled execution should return an error")
}

func TestExecute_CleansUpFinishedProcesses(t *testing.T) {
	w := newTestWizard(t)
	rep := &mocks.Reporter{}
	s := step.NewRunStep("echo", "echo hello", 5*time.Second, true)
	req := newTestReq(s)

	err := w.Execute(context.Background(), req, rep)
	require.NoError(t, err)

	assert.Equal(t, 0, w.runtime.Count(), "finished processes should be cleaned up after Execute")
}

func BenchmarkExecute_MultipleSteps(b *testing.B) {
	w, err := New()
	if err != nil {
		b.Fatal(err)
	}
	steps := make([]step.Step, 10)
	for i := range steps {
		steps[i] = step.NewDependenciesStep("dep")
	}
	req := ExecutionRequest{
		Namespace: domain.Namespace("bench/user/repo/arrow"),
		Method:    "execute",
		Variables: map[string]string{},
		Steps:     steps,
		WorkDir:   "/tmp",
	}
	rep := &mocks.Reporter{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.Execute(context.Background(), req, rep)
		rep.Started = rep.Started[:0]
		rep.Completed = rep.Completed[:0]
		rep.Failed = rep.Failed[:0]
	}
}
