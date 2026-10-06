package runtimeinternal_test

import (
	"context"
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
)

const probeNs = domain.Namespace("github.com/user/chat@v1.0.0")

// newTestRuntime returns a runtime aggregate whose ns is running execution
// testExecutionID.
func newTestRuntime(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	ax := newTestAsynxRuntimeForHooks(t)
	seedRunningRuntimeForHooks(t, ax, probeNs)
	return ax
}

var testSurface = domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen, Path: "/"}

// newTestRuntimeWithSurface is newTestRuntime with testSurface open on the
// running execution, as the drain leaves it before it starts the probe.
func newTestRuntimeWithSurface(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	ax := newTestRuntime(t)
	_, err := ax.Send(t.Context(), commands.SetSurface{
		Namespace:   probeNs,
		ExecutionID: testExecutionID,
		Surface:     testSurface,
	})
	require.NoError(t, err)
	return ax
}

func TestProbeSurface_FlipsReadyOnceServing(t *testing.T) {
	ax := newTestRuntimeWithSurface(t)
	calls := 0
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(context.Context, domain.Namespace, domainRuntime.Surface) bool {
			calls++
			return calls >= 3
		},
	}
	runtimeinternal.ProbeSurface(t.Context(), hooks, ax, probeNs.String(), testExecutionID, testSurface, time.Millisecond, time.Second)

	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	require.NotNil(t, rt.Execution.Surface)
	require.True(t, rt.Execution.Surface.Ready)
	require.Equal(t, 3, calls)
}

// runProbe runs the probe in the background and fails the test if it has not
// returned within a few seconds.
func runProbe(
	t *testing.T,
	run func(),
) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); run() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the probe never returned")
	}
}

func TestProbeSurface_StopsWhenTheExecutionEnds(t *testing.T) {
	ax := newTestRuntimeWithSurface(t)
	var calls atomic.Int32
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(ctx context.Context, _ domain.Namespace, _ domainRuntime.Surface) bool {
			if calls.Add(1) == 3 {
				_, err := ax.Send(ctx, commands.EndExecution{
					Namespace: probeNs, ExecutionID: testExecutionID, Outcome: domainRuntime.ExecutionOutcomeSuccess,
				})
				assert.NoError(t, err)
			}
			return false
		},
	}

	runProbe(t, func() {
		runtimeinternal.ProbeSurface(t.Context(), hooks, ax, probeNs.String(), testExecutionID,
			testSurface, time.Millisecond, 200*time.Millisecond)
	})

	assert.Equal(t, int32(3), calls.Load(), "no dial after the execution ended")
}

func TestProbeSurface_StopsWhenTheExecutionIsReplaced(t *testing.T) {
	ax := newTestRuntimeWithSurface(t)
	var calls atomic.Int32
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(ctx context.Context, _ domain.Namespace, _ domainRuntime.Surface) bool {
			if calls.Add(1) == 3 {
				_, err := ax.Send(ctx, commands.BeginStop{
					Namespace:   probeNs,
					ExecutionID: "stop",
					Steps:       domainStep.StepList{domainStep.NewSignalStep("stop", domainStep.SignalKindGraceful, "10s", false)},
				})
				assert.NoError(t, err)
			}
			return false
		},
	}

	runProbe(t, func() {
		runtimeinternal.ProbeSurface(t.Context(), hooks, ax, probeNs.String(), testExecutionID,
			testSurface, time.Millisecond, 200*time.Millisecond)
	})

	assert.Equal(t, int32(3), calls.Load(), "no dial after another execution took over")
	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	assert.Equal(t, "stop", rt.Execution.ID)
	assert.Nil(t, rt.Execution.Surface)
}

func TestProbeSurface_KeepsPollingWhileTheExecutionIsCurrent(t *testing.T) {
	ax := newTestRuntimeWithSurface(t)
	var calls atomic.Int32
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(context.Context, domain.Namespace, domainRuntime.Surface) bool {
			return calls.Add(1) >= 40
		},
	}

	start := time.Now()
	runProbe(t, func() {
		runtimeinternal.ProbeSurface(t.Context(), hooks, ax, probeNs.String(), testExecutionID,
			testSurface, time.Millisecond, 2*time.Millisecond)
	})

	require.Greater(t, time.Since(start), 2*time.Millisecond)
	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	require.NotNil(t, rt.Execution.Surface)
	assert.True(t, rt.Execution.Surface.Ready, "a slow arrow still becomes ready")
}

func TestProbeSurface_StopsWhenTheSurfaceIsCleared(t *testing.T) {
	ax := newTestRuntimeWithSurface(t)
	var calls atomic.Int32
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(ctx context.Context, _ domain.Namespace, _ domainRuntime.Surface) bool {
			if calls.Add(1) == 3 {
				_, err := ax.Send(ctx, commands.ClearSurface{Namespace: probeNs, ExecutionID: testExecutionID})
				assert.NoError(t, err)
			}
			return false
		},
	}

	runProbe(t, func() {
		runtimeinternal.ProbeSurface(t.Context(), hooks, ax, probeNs.String(), testExecutionID,
			testSurface, time.Millisecond, 200*time.Millisecond)
	})

	assert.Equal(t, int32(3), calls.Load(), "no dial after the run that opened the surface exited")
	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	assert.Equal(t, testExecutionID, rt.Execution.ID)
	assert.Nil(t, rt.Execution.Surface)
}

func TestProbeSurface_LeavesALaterRunsSurfaceAlone(t *testing.T) {
	ax := newTestRuntimeWithSurface(t)
	later := domainRuntime.Surface{Mode: domainRuntime.SurfaceModeStatic, Path: "/docs", Dir: "/srv/docs", Ready: true}
	var calls atomic.Int32
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(ctx context.Context, _ domain.Namespace, _ domainRuntime.Surface) bool {
			if calls.Add(1) == 2 {
				_, err := ax.Send(ctx, commands.SetSurface{Namespace: probeNs, ExecutionID: testExecutionID, Surface: later})
				assert.NoError(t, err)
			}
			return false
		},
	}

	runProbe(t, func() {
		runtimeinternal.ProbeSurface(t.Context(), hooks, ax, probeNs.String(), testExecutionID,
			testSurface, time.Millisecond, 200*time.Millisecond)
	})

	assert.Equal(t, int32(2), calls.Load())
	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	assert.Equal(t, &later, rt.Execution.Surface)
}

func TestNextProbeInterval_BacksOffToTheCeiling(t *testing.T) {
	got := []time.Duration{}
	d := 250 * time.Millisecond
	for range 6 {
		d = runtimeinternal.NextProbeInterval(d, 2*time.Second)
		got = append(got, d)
	}
	assert.Equal(t, []time.Duration{
		500 * time.Millisecond, time.Second, 2 * time.Second, 2 * time.Second, 2 * time.Second, 2 * time.Second,
	}, got)
	assert.Equal(t, 2*time.Second, runtimeinternal.NextProbeInterval(1500*time.Millisecond, 2*time.Second))
}

func TestProbeSurface_AlreadyReadyIsNoOp(t *testing.T) {
	called := false
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(context.Context, domain.Namespace, domainRuntime.Surface) bool { called = true; return true },
	}

	runtimeinternal.ProbeSurface(t.Context(), hooks, nil, "a/b", testExecutionID, domainRuntime.Surface{Ready: true}, time.Millisecond, time.Second)

	require.False(t, called)
}

func TestProbeSurface_NoReadyHookIsNoOp(t *testing.T) {
	require.NotPanics(t, func() {
		runtimeinternal.ProbeSurface(t.Context(), runtimeinternal.CatalogHooks{}, nil, "a/b", testExecutionID,
			domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen}, time.Millisecond, time.Second)
	})
}

func TestProbeSurface_SupersededReadyIsDropped(t *testing.T) {
	ns := domain.Namespace("github.com/user/chat-superseded@v1.0.0")
	ax := newTestAsynxRuntimeForHooks(t)
	runID, stopID := seedStoppedOverRun(t, ax, ns)
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(context.Context, domain.Namespace, domainRuntime.Surface) bool { return true },
	}

	runtimeinternal.ProbeSurface(t.Context(), hooks, ax, ns.String(), runID,
		domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen}, time.Millisecond, time.Second)

	rt, err := ax.Get(t.Context(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, stopID, rt.Execution.ID)
	assert.Nil(t, rt.Execution.Surface)
}

func TestDrainExecution_SurfaceEvent_SetsSurfaceOnExecution(t *testing.T) {
	ax := newTestRuntime(t)
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{
		Kind:    wizard.EventKindSurface,
		Surface: &domainRuntime.Surface{Mode: domainRuntime.SurfaceModeStatic, Path: "/", Dir: "/srv/dist"},
	})
	exec.emit(wizard.Event{Kind: wizard.EventKindSurface})
	exec.close()

	superseded := runtimeinternal.DrainEvents(t.Context(), exec, probeNs.String(), testExecutionID, runtimeinternal.CatalogHooks{}, ax)
	require.False(t, superseded)
	ax.WaitPublish()

	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	require.NotNil(t, rt.Execution.Surface)
	assert.Equal(t, domainRuntime.SurfaceModeStatic, rt.Execution.Surface.Mode)
	assert.Equal(t, "/srv/dist", rt.Execution.Surface.Dir)
}

func TestDrainExecution_SurfaceEvent_StartsTheProbe(t *testing.T) {
	ax := newTestRuntime(t)
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(context.Context, domain.Namespace, domainRuntime.Surface) bool { return true },
	}
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{
		Kind:    wizard.EventKindSurface,
		Surface: &domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen, Path: "/"},
	})
	exec.close()

	runtimeinternal.DrainEvents(t.Context(), exec, probeNs.String(), testExecutionID, hooks, ax)

	require.Eventually(t, func() bool {
		rt, err := ax.Get(t.Context(), probeNs.String())
		return err == nil && rt.Execution.Surface != nil && rt.Execution.Surface.Ready
	}, 5*time.Second, 10*time.Millisecond, "the readiness probe never flipped Ready")
}

func TestDrainExecution_SurfaceEventAfterTakeover_IsDropped(t *testing.T) {
	ns := domain.Namespace("github.com/user/chat-takeover@v1.0.0")
	ax := newTestAsynxRuntimeForHooks(t)
	runID, _ := seedStoppedOverRun(t, ax, ns)
	var probes atomic.Int32
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(context.Context, domain.Namespace, domainRuntime.Surface) bool {
			probes.Add(1)
			return true
		},
	}
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeCancelled)
	exec.emit(wizard.Event{
		Kind:    wizard.EventKindSurface,
		Surface: &domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen, Path: "/"},
	})
	exec.close()

	superseded := runtimeinternal.DrainEvents(t.Context(), exec, ns.String(), runID, hooks, ax)

	require.True(t, superseded)
	rt, err := ax.Get(t.Context(), ns.String())
	require.NoError(t, err)
	assert.Nil(t, rt.Execution.Surface)
	assert.Zero(t, probes.Load(), "a superseded surface is never probed")
}

func TestDrainExecution_SurfaceClosedEvent_ClearsSurfaceAndReleasesSocketOnce(t *testing.T) {
	ax := newTestRuntime(t)
	var closed []domain.Namespace
	hooks := runtimeinternal.CatalogHooks{
		MarkLastUsed: noopMarkLastUsed,
		CloseSurface: func(ns domain.Namespace) { closed = append(closed, ns) },
	}
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{Kind: wizard.EventKindSurface, Surface: &testSurface})
	exec.emit(wizard.Event{Kind: wizard.EventKindSurfaceClosed})
	exec.close()

	superseded := runtimeinternal.DrainEvents(t.Context(), exec, probeNs.String(), testExecutionID, hooks, ax)

	assert.False(t, superseded)
	assert.Equal(t, []domain.Namespace{probeNs}, closed)
	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	assert.Nil(t, rt.Execution.Surface)
	assert.NotNil(t, rt.Execution, "the execution carries on after its run's surface closes")
}

func TestDrainExecution_SurfaceClosedBeforeEnd_IsNotReleasedAgainAtEnd(t *testing.T) {
	ax := newTestRuntime(t)
	closed := 0
	hooks := runtimeinternal.CatalogHooks{
		MarkLastUsed: noopMarkLastUsed,
		CloseSurface: func(domain.Namespace) { closed++ },
	}
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{Kind: wizard.EventKindSurface, Surface: &testSurface})
	exec.emit(wizard.Event{Kind: wizard.EventKindSurfaceClosed})
	exec.close()

	runtimeinternal.DrainExecutionWithHooks(t.Context(), exec, probeNs.String(), testExecutionID, domain.MethodExecute, hooks, ax)

	assert.Equal(t, 1, closed)
}

func TestDrainExecution_SecondRunSurfaceIsReleasedAtEndWhenItsCloseWasLost(t *testing.T) {
	ax := newTestRuntime(t)
	closed := 0
	hooks := runtimeinternal.CatalogHooks{
		MarkLastUsed: noopMarkLastUsed,
		CloseSurface: func(domain.Namespace) { closed++ },
	}
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{Kind: wizard.EventKindSurface, Surface: &testSurface})
	exec.emit(wizard.Event{Kind: wizard.EventKindSurfaceClosed})
	exec.emit(wizard.Event{Kind: wizard.EventKindSurface, Surface: &testSurface})
	exec.close()

	runtimeinternal.DrainExecutionWithHooks(t.Context(), exec, probeNs.String(), testExecutionID, domain.MethodExecute, hooks, ax)

	assert.Equal(t, 2, closed)
}

func TestDrainExecution_SurfaceClosedAfterTakeover_ClosesNothing(t *testing.T) {
	ns := domain.Namespace("github.com/user/chat-closed-takeover@v1.0.0")
	ax := newTestAsynxRuntimeForHooks(t)
	runID, _ := seedStoppedOverRun(t, ax, ns)
	closed := 0
	hooks := runtimeinternal.CatalogHooks{
		CloseSurface: func(domain.Namespace) { closed++ },
	}
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeCancelled)
	exec.emit(wizard.Event{Kind: wizard.EventKindSurfaceClosed})
	exec.close()

	superseded := runtimeinternal.DrainEvents(t.Context(), exec, ns.String(), runID, hooks, ax)

	assert.True(t, superseded)
	assert.Zero(t, closed, "a superseded run must not release the socket of whoever replaced it")
}

func TestDrainExecution_SurfaceClosedWithoutCloseHook_NoPanic(t *testing.T) {
	ax := newTestRuntimeWithSurface(t)
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{Kind: wizard.EventKindSurfaceClosed})
	exec.close()

	require.NotPanics(t, func() {
		runtimeinternal.DrainEvents(t.Context(), exec, probeNs.String(), testExecutionID, runtimeinternal.CatalogHooks{}, ax)
	})
}

func TestFinishExecution_ClosesSurfaceOnceOnSuccessfulEnd(t *testing.T) {
	ax := newTestRuntime(t)
	var closed []domain.Namespace
	hooks := runtimeinternal.CatalogHooks{
		MarkLastUsed: noopMarkLastUsed,
		CloseSurface: func(ns domain.Namespace) { closed = append(closed, ns) },
	}
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecutionWithHooks(t.Context(), exec, probeNs.String(), testExecutionID, domain.MethodExecute, hooks, ax)

	assert.Equal(t, []domain.Namespace{probeNs}, closed)
}

func TestFinishExecution_SupersededRunLeavesSurfaceOpen(t *testing.T) {
	ns := domain.Namespace("github.com/user/chat-keep@v1.0.0")
	ax := newTestAsynxRuntimeForHooks(t)
	runID, _ := seedStoppedOverRun(t, ax, ns)
	closed := 0
	hooks := runtimeinternal.CatalogHooks{
		CloseSurface: func(domain.Namespace) { closed++ },
	}
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeCancelled)
	exec.close()

	runtimeinternal.DrainExecutionWithHooks(t.Context(), exec, ns.String(), runID, domain.MethodExecute, hooks, ax)

	assert.Zero(t, closed)
}

func TestFinishExecution_InstallSurfaceIsClosedAndDroppedAtEnd(t *testing.T) {
	testCases := []struct {
		name    string
		outcome domainRuntime.ExecutionOutcome
	}{
		{name: "success", outcome: domainRuntime.ExecutionOutcomeSuccess},
		{name: "failure", outcome: domainRuntime.ExecutionOutcomeFailed},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := newTestRuntime(t)
			var closed []domain.Namespace
			hooks := runtimeinternal.CatalogHooks{
				MarkInstalled: noopMarkInstalled,
				CloseSurface:  func(ns domain.Namespace) { closed = append(closed, ns) },
			}
			exec := newFakeExecution(tc.outcome)
			exec.emit(wizard.Event{
				Kind:    wizard.EventKindSurface,
				Surface: &domainRuntime.Surface{Mode: domainRuntime.SurfaceModeStatic, Path: "/", Ready: true},
			})
			exec.close()

			runtimeinternal.DrainExecutionWithHooks(t.Context(), exec, probeNs.String(), testExecutionID, domain.MethodInstall, hooks, ax)

			assert.Equal(t, []domain.Namespace{probeNs}, closed)
			rt, err := ax.Get(t.Context(), probeNs.String())
			require.NoError(t, err)
			assert.Nil(t, rt.Execution, "an ended install keeps no surface for the proxy to serve")
		})
	}
}
