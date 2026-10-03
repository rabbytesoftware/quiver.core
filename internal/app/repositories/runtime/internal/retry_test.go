package runtimeinternal_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runtimeinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

var errBoom = errors.New("boom")

func mismatchExecution() *fakeExecution {
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeFailed)
	exec.emit(wizard.Event{Kind: wizard.EventKindStepStarted, StepIndex: 0})
	exec.emit(wizard.Event{
		Kind:      wizard.EventKindStepFailed,
		StepIndex: 0,
		Err:       fmt.Errorf("download: checksum: /x: %w", wizard.ErrChecksumMismatch),
	})
	exec.close()
	return exec
}

func failingExecution(err error) *fakeExecution {
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeFailed)
	exec.emit(wizard.Event{Kind: wizard.EventKindStepFailed, StepIndex: 0, Err: err})
	exec.close()
	return exec
}

func succeedingExecution() *fakeExecution {
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{Kind: wizard.EventKindStepStarted, StepIndex: 0})
	exec.emit(wizard.Event{Kind: wizard.EventKindStepCompleted, StepIndex: 0})
	exec.close()
	return exec
}

func freshSteps() []domainStep.Step {
	return []domainStep.Step{domainStep.NewFetchStep("fresh", "https://x/a", "a", "sha256:bb", "1m", true)}
}

type retryFixture struct {
	ns        domain.Namespace
	ax        asynx.Asynx[domainRuntime.ArrowRuntime]
	hooks     runtimeinternal.CatalogHooks
	wizard    *mocks.Wizard
	refreshes atomic.Int32
	starts    atomic.Int32
	installs  atomic.Int32
	retryRuns []wizard.RunRequest
	next      []wizard.Execution
}

func newRetryFixture(t *testing.T) *retryFixture {
	t.Helper()
	f := &retryFixture{
		ns: domain.Namespace("github.com/user/repo@v1.0.0"),
		ax: newTestAsynxRuntimeForHooks(t),
	}
	f.hooks = runtimeinternal.CatalogHooks{
		MarkInstalled:   func(context.Context, domain.Namespace, time.Time) error { f.installs.Add(1); return nil },
		MarkUninstalled: noopMarkUninstalled,
		MarkLastUsed:    noopMarkLastUsed,
		RefreshManifest: func(context.Context, domain.Namespace, string) error { f.refreshes.Add(1); return nil },
		Reassemble: func(context.Context, domain.Namespace, string, map[string]string) ([]domainStep.Step, map[string]string, error) {
			return freshSteps(), map[string]string{"K": "v"}, nil
		},
	}
	f.wizard = &mocks.Wizard{StartFn: func(_ context.Context, req wizard.RunRequest) wizard.Execution {
		idx := int(f.starts.Add(1)) - 1
		f.retryRuns = append(f.retryRuns, req)
		return f.next[idx]
	}}
	return f
}

func (f *retryFixture) supervise(
	ctx context.Context,
	first wizard.Execution,
	method string,
) {
	req := wizard.RunRequest{
		Namespace: f.ns,
		Method:    method,
		Steps:     domainStep.StepList{testStep()},
		Variables: map[string]string{"K": "v"},
		WorkDir:   "/tmp/w",
	}
	runtimeinternal.SuperviseExecution(ctx, first, req, testExecutionID, f.hooks, f.wizard, f.ax)
	f.ax.WaitPublish()
}

func (f *retryFixture) runtime(
	t *testing.T,
) *domainRuntime.ArrowRuntime {
	t.Helper()
	got, err := f.ax.Get(context.Background(), f.ns.String())
	require.NoError(t, err)
	return &got
}

func TestSuperviseExecution_InstallChecksumMismatch_RefreshesAndRetriesOnce(t *testing.T) {
	f := newRetryFixture(t)
	seedInstallingRuntimeForHooks(t, f.ax, f.ns)
	f.next = []wizard.Execution{succeedingExecution()}

	f.supervise(context.Background(), mismatchExecution(), domain.MethodInstall)

	assert.EqualValues(t, 1, f.refreshes.Load())
	assert.EqualValues(t, 1, f.starts.Load())
	assert.EqualValues(t, 1, f.installs.Load())
	require.Len(t, f.retryRuns, 1)
	assert.Equal(t, freshSteps(), f.retryRuns[0].Steps)
	assert.Equal(t, "/tmp/w", f.retryRuns[0].WorkDir)
	assert.Equal(t, map[string]string{"K": "v"}, f.retryRuns[0].Variables)
	got := f.runtime(t)
	assert.Equal(t, domain.ArrowStateReady, got.State)
	require.NotNil(t, got.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeSuccess, got.LastReturn.Outcome)
	require.Len(t, got.LastReturn.Steps, 1)
	assert.Equal(t, freshSteps()[0], got.LastReturn.Steps[0].Step)
	assert.Equal(t, domainRuntime.StepStatusCompleted, got.LastReturn.Steps[0].Status)
}

func TestSuperviseExecution_UpdateChecksumMismatch_RefreshesAndRetriesOnce(t *testing.T) {
	f := newRetryFixture(t)
	seedInstallingRuntimeForHooks(t, f.ax, f.ns)
	_, err := f.ax.Send(context.Background(), commands.EndExecution{
		Namespace: f.ns, ExecutionID: testExecutionID, Outcome: domainRuntime.ExecutionOutcomeSuccess,
	})
	require.NoError(t, err)
	_, err = f.ax.Send(context.Background(), commands.BeginUpdate{
		Namespace: f.ns, ExecutionID: testExecutionID, Steps: domainStep.StepList{testStep()},
	})
	require.NoError(t, err)
	f.next = []wizard.Execution{succeedingExecution()}

	f.supervise(context.Background(), mismatchExecution(), domain.MethodUpdate)

	assert.EqualValues(t, 1, f.refreshes.Load())
	assert.EqualValues(t, 1, f.starts.Load())
	assert.Equal(t, domain.ArrowStateReady, f.runtime(t).State)
}

func TestSuperviseExecution_ChecksumMismatchTwice_FailsAfterExactlyOneRetry(t *testing.T) {
	f := newRetryFixture(t)
	seedInstallingRuntimeForHooks(t, f.ax, f.ns)
	f.next = []wizard.Execution{mismatchExecution()}

	f.supervise(context.Background(), mismatchExecution(), domain.MethodInstall)

	assert.EqualValues(t, 1, f.refreshes.Load())
	assert.EqualValues(t, 1, f.starts.Load())
	assert.Zero(t, f.installs.Load())
	got := f.runtime(t)
	assert.Equal(t, domain.ArrowStateAbsent, got.State)
	require.NotNil(t, got.LastReturn)
	assert.Equal(t, domainRuntime.ExecutionOutcomeFailed, got.LastReturn.Outcome)
	require.NotNil(t, got.LastReturn.Steps[0].Error)
}

func TestSuperviseExecution_NoRetry(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	testCases := []struct {
		name   string
		method string
		first  func() *fakeExecution
		ctx    context.Context
		tweak  func(f *retryFixture)
	}{
		{
			name:   "non checksum failure",
			method: domain.MethodInstall,
			first:  func() *fakeExecution { return failingExecution(errBoom) },
		},
		{
			name:   "method is not install or update",
			method: domain.MethodExecute,
			first:  mismatchExecution,
		},
		{
			name:   "cancelled outcome",
			method: domain.MethodInstall,
			first: func() *fakeExecution {
				exec := newFakeExecution(domainRuntime.ExecutionOutcomeCancelled)
				exec.emit(wizard.Event{Kind: wizard.EventKindStepFailed, Err: wizard.ErrChecksumMismatch})
				exec.close()
				return exec
			},
		},
		{
			name:   "identical manifest",
			method: domain.MethodInstall,
			first:  mismatchExecution,
			tweak: func(f *retryFixture) {
				f.hooks.Reassemble = func(context.Context, domain.Namespace, string, map[string]string) ([]domainStep.Step, map[string]string, error) {
					return []domainStep.Step{testStep()}, map[string]string{"K": "v"}, nil
				}
			},
		},
		{
			name:   "refresh fails",
			method: domain.MethodInstall,
			first:  mismatchExecution,
			tweak: func(f *retryFixture) {
				f.hooks.RefreshManifest = func(context.Context, domain.Namespace, string) error { f.refreshes.Add(1); return errBoom }
			},
		},
		{
			name:   "reassemble fails",
			method: domain.MethodInstall,
			first:  mismatchExecution,
			tweak: func(f *retryFixture) {
				f.hooks.Reassemble = func(context.Context, domain.Namespace, string, map[string]string) ([]domainStep.Step, map[string]string, error) {
					return nil, nil, errBoom
				}
			},
		},
		{
			name:   "no refresh hook",
			method: domain.MethodInstall,
			first:  mismatchExecution,
			tweak:  func(f *retryFixture) { f.hooks.RefreshManifest = nil },
		},
		{
			name:   "no reassemble hook",
			method: domain.MethodInstall,
			first:  mismatchExecution,
			tweak:  func(f *retryFixture) { f.hooks.Reassemble = nil },
		},
		{
			name:   "context cancelled",
			method: domain.MethodInstall,
			first:  mismatchExecution,
			ctx:    cancelled,
		},
		{
			name:   "execution ended before the restart",
			method: domain.MethodInstall,
			first:  mismatchExecution,
			tweak: func(f *retryFixture) {
				f.hooks.RefreshManifest = func(ctx context.Context, ns domain.Namespace, _ string) error {
					f.refreshes.Add(1)
					_, err := f.ax.Send(ctx, commands.EndExecution{
						Namespace: ns, ExecutionID: testExecutionID, Outcome: domainRuntime.ExecutionOutcomeCancelled,
					})
					return err
				}
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRetryFixture(t)
			seedInstallingRuntimeForHooks(t, f.ax, f.ns)
			if tc.tweak != nil {
				tc.tweak(f)
			}
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			f.supervise(ctx, tc.first(), tc.method)

			assert.Zero(t, f.starts.Load())
			assert.Zero(t, f.installs.Load())
		})
	}
}

func TestSuperviseExecution_SameStepsButNewVariables_RetriesWithTheNewVariables(t *testing.T) {
	f := newRetryFixture(t)
	seedInstallingRuntimeForHooks(t, f.ax, f.ns)
	f.next = []wizard.Execution{succeedingExecution()}
	f.hooks.Reassemble = func(context.Context, domain.Namespace, string, map[string]string) ([]domainStep.Step, map[string]string, error) {
		return []domainStep.Step{testStep()}, map[string]string{"K": "resolved again"}, nil
	}

	f.supervise(context.Background(), mismatchExecution(), domain.MethodInstall)

	require.Len(t, f.retryRuns, 1)
	assert.Equal(t, map[string]string{"K": "resolved again"}, f.retryRuns[0].Variables)
}

func TestSuperviseExecution_SuccessfulRun_DoesNotTouchTheManifest(t *testing.T) {
	f := newRetryFixture(t)
	seedInstallingRuntimeForHooks(t, f.ax, f.ns)

	f.supervise(context.Background(), succeedingExecution(), domain.MethodInstall)

	assert.Zero(t, f.refreshes.Load())
	assert.Zero(t, f.starts.Load())
	assert.EqualValues(t, 1, f.installs.Load())
}

func TestSuperviseExecution_SupersededRun_DoesNotRetry(t *testing.T) {
	f := newRetryFixture(t)
	seedInstallingRuntimeForHooks(t, f.ax, f.ns)

	req := wizard.RunRequest{Namespace: f.ns, Method: domain.MethodInstall}
	runtimeinternal.SuperviseExecution(context.Background(), mismatchExecution(), req, "another-execution", f.hooks, f.wizard, f.ax)

	assert.Zero(t, f.refreshes.Load())
	assert.Zero(t, f.starts.Load())
}
