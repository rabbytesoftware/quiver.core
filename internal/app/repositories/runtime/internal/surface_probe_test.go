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
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
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

func TestProbeSurface_FlipsReadyOnceServing(t *testing.T) {
	ax := newTestRuntime(t)
	calls := 0
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(context.Context, domain.Namespace, domainRuntime.Surface) bool {
			calls++
			return calls >= 3
		},
	}
	s := domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen, Path: "/"}

	runtimeinternal.ProbeSurface(t.Context(), hooks, ax, probeNs.String(), testExecutionID, s, time.Millisecond, time.Second)

	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	require.NotNil(t, rt.Execution.Surface)
	require.True(t, rt.Execution.Surface.Ready)
	require.Equal(t, 3, calls)
}

func TestProbeSurface_GivesUpAfterTimeout(t *testing.T) {
	ax := newTestRuntime(t)
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(context.Context, domain.Namespace, domainRuntime.Surface) bool { return false },
	}

	runtimeinternal.ProbeSurface(t.Context(), hooks, ax, probeNs.String(), testExecutionID,
		domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen}, time.Millisecond, 20*time.Millisecond)

	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	require.False(t, rt.Execution.Surface != nil && rt.Execution.Surface.Ready)
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
