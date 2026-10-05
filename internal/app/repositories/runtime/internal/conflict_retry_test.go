package runtimeinternal_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runtimeinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

const persistentConflict = -1

// conflictingRuntime fails the first sends of chosen command types with the
// error a losing concurrent writer gets from the sqlite store, then lets them
// through to the real aggregate.
type conflictingRuntime struct {
	asynx.Asynx[domainRuntime.ArrowRuntime]

	mu        sync.Mutex
	conflicts map[string]int
	sends     map[string]int
}

func withConflicts(
	ax asynx.Asynx[domainRuntime.ArrowRuntime],
	conflicts map[string]int,
) *conflictingRuntime {
	return &conflictingRuntime{Asynx: ax, conflicts: conflicts, sends: map[string]int{}}
}

func (c *conflictingRuntime) Send(
	ctx context.Context,
	cmd asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	name := fmt.Sprintf("%T", cmd)
	c.mu.Lock()
	c.sends[name]++
	remaining := c.conflicts[name]
	if remaining > 0 {
		c.conflicts[name]--
	}
	c.mu.Unlock()
	if remaining != 0 {
		storeErr := fmt.Errorf("%w: version conflict (events:%s, v14)", asynxModels.ErrPipelineFailed, cmd.AggregateID())
		return asynxModels.Event[domainRuntime.ArrowRuntime]{}, fmt.Errorf("%w: %w", asynxModels.ErrPipelineFailed, storeErr)
	}
	return c.Asynx.Send(ctx, cmd)
}

func (c *conflictingRuntime) sendsOf(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sends[name]
}

// captureLogs routes the default logger into a buffer for the test's duration.
func captureLogs(t *testing.T) *lockedBuffer {
	t.Helper()
	buf := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) count(substr string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Count(b.buf.String(), substr)
}

func drainOnly(t *testing.T, ax asynx.Asynx[domainRuntime.ArrowRuntime], events ...wizard.Event) bool {
	t.Helper()
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	for _, evt := range events {
		exec.emit(evt)
	}
	exec.close()
	return runtimeinternal.DrainEvents(t.Context(), exec, probeNs.String(), testExecutionID, runtimeinternal.CatalogHooks{}, ax)
}

func TestRecordReady_SurvivesATransientVersionConflict(t *testing.T) {
	ax := withConflicts(newTestRuntime(t), map[string]int{"commands.SetSurface": 2})
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(context.Context, domain.Namespace, domainRuntime.Surface) bool { return true },
	}

	runProbe(t, func() {
		runtimeinternal.ProbeSurface(t.Context(), hooks, ax, probeNs.String(), testExecutionID,
			domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen}, time.Millisecond, time.Second)
	})

	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	require.NotNil(t, rt.Execution.Surface)
	assert.True(t, rt.Execution.Surface.Ready)
	assert.Equal(t, 3, ax.sendsOf("commands.SetSurface"))
}

func TestSendPID_SurvivesATransientVersionConflict(t *testing.T) {
	ax := withConflicts(newTestRuntime(t), map[string]int{"commands.RecordPID": 2})

	superseded := drainOnly(t, ax, wizard.Event{Kind: wizard.EventKindPID, PID: 22502})

	assert.False(t, superseded)
	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	assert.Equal(t, 22502, rt.Execution.PID)
	assert.Equal(t, 3, ax.sendsOf("commands.RecordPID"))
}

func TestSendStep_SurvivesATransientVersionConflict(t *testing.T) {
	ax := withConflicts(newTestRuntime(t), map[string]int{"commands.AdvanceStep": 2})

	superseded := drainOnly(t, ax, wizard.Event{Kind: wizard.EventKindStepStarted, StepIndex: 0})

	assert.False(t, superseded)
	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	assert.Equal(t, domainRuntime.StepStatusRunning, rt.Execution.Steps[0].Status)
	assert.Equal(t, 3, ax.sendsOf("commands.AdvanceStep"))
}

func TestSendSurface_SurvivesATransientVersionConflict(t *testing.T) {
	ax := withConflicts(newTestRuntime(t), map[string]int{"commands.SetSurface": 1})
	s := domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen, Path: "/", Ready: true}

	superseded := drainOnly(t, ax, wizard.Event{Kind: wizard.EventKindSurface, Surface: &s})

	assert.False(t, superseded)
	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	require.NotNil(t, rt.Execution.Surface)
	assert.Equal(t, "/", rt.Execution.Surface.Path)
}

func TestSendEndExecution_SurvivesATransientVersionConflict(t *testing.T) {
	ax := withConflicts(newTestRuntime(t), map[string]int{"commands.EndExecution": 2})
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.close()

	runtimeinternal.DrainExecution(t.Context(), exec, probeNs.String(), testExecutionID, domain.MethodExecute,
		noopMarkInstalled, noopMarkUninstalled, noopMarkLastUsed, ax)

	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	assert.Nil(t, rt.Execution)
	assert.Equal(t, domain.ArrowStateReady, rt.State)
}

func TestSendPID_PersistentVersionConflict_GivesUpBoundedAndLogsOnce(t *testing.T) {
	logs := captureLogs(t)
	ax := withConflicts(newTestRuntime(t), map[string]int{"commands.RecordPID": persistentConflict})

	start := time.Now()
	superseded := drainOnly(t, ax, wizard.Event{Kind: wizard.EventKindPID, PID: 42})

	assert.False(t, superseded)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.Equal(t, runtimeinternal.ConflictRetryAttempts, ax.sendsOf("commands.RecordPID"))
	assert.Equal(t, 1, logs.count("runtime: sendPID failed"))
	rt, err := ax.Get(t.Context(), probeNs.String())
	require.NoError(t, err)
	assert.Zero(t, rt.Execution.PID)
}

func TestRecordReady_PersistentVersionConflict_GivesUpBoundedAndLogsOnce(t *testing.T) {
	logs := captureLogs(t)
	ax := withConflicts(newTestRuntime(t), map[string]int{"commands.SetSurface": persistentConflict})
	hooks := runtimeinternal.CatalogHooks{
		SurfaceReady: func(context.Context, domain.Namespace, domainRuntime.Surface) bool { return true },
	}

	runProbe(t, func() {
		runtimeinternal.ProbeSurface(t.Context(), hooks, ax, probeNs.String(), testExecutionID,
			domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen}, time.Millisecond, time.Second)
	})

	assert.Equal(t, runtimeinternal.ConflictRetryAttempts, ax.sendsOf("commands.SetSurface"))
	assert.Equal(t, 1, logs.count("runtime: recording surface ready failed"))
}

func TestSendPID_Superseded_IsNotRetried(t *testing.T) {
	ax := withConflicts(newTestRuntime(t), map[string]int{})
	exec := newFakeExecution(domainRuntime.ExecutionOutcomeSuccess)
	exec.emit(wizard.Event{Kind: wizard.EventKindPID, PID: 42})
	exec.close()

	superseded := runtimeinternal.DrainEvents(t.Context(), exec, probeNs.String(), "an-older-execution",
		runtimeinternal.CatalogHooks{}, ax)

	assert.True(t, superseded)
	assert.Equal(t, 1, ax.sendsOf("commands.RecordPID"))
}

func TestSendRetryingConflicts_StopsWaitingWhenTheContextEnds(t *testing.T) {
	ax := withConflicts(newTestRuntime(t), map[string]int{"commands.RecordPID": persistentConflict})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := runtimeinternal.SendRetryingConflicts(ctx, ax, commands.RecordPID{
		Namespace: probeNs, ExecutionID: testExecutionID, PID: 42,
	})

	require.ErrorIs(t, err, asynxModels.ErrPipelineFailed)
	assert.Equal(t, 1, ax.sendsOf("commands.RecordPID"))
}

func TestSendRetryingConflicts_DoesNotRetryAWriteThatLanded(t *testing.T) {
	ax := withConflicts(newTestRuntime(t), map[string]int{})
	landed := 0
	closed := &dispatcherClosedRuntime{Asynx: ax, onSend: func() { landed++ }}

	err := runtimeinternal.SendRetryingConflicts(t.Context(), closed, commands.RecordPID{
		Namespace: probeNs, ExecutionID: testExecutionID, PID: 42,
	})

	require.ErrorIs(t, err, asynxModels.ErrDispatcherClosed)
	assert.Equal(t, 1, landed)
}

// dispatcherClosedRuntime commits the command and then reports the error
// asynx returns when only the post-write dispatch failed.
type dispatcherClosedRuntime struct {
	asynx.Asynx[domainRuntime.ArrowRuntime]
	onSend func()
}

func (d *dispatcherClosedRuntime) Send(
	ctx context.Context,
	cmd asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	d.onSend()
	evt, err := d.Asynx.Send(ctx, cmd)
	if err != nil {
		return evt, err
	}
	return evt, fmt.Errorf("%w: %w", asynxModels.ErrPipelineFailed, asynxModels.ErrDispatcherClosed)
}

// TestConcurrentWriters_DrainAndProbe_NeitherIsLost races the drain
// goroutine's step and PID writes against the probe's ready write on one
// aggregate, the interleaving the tcp daemon hit.
func TestConcurrentWriters_DrainAndProbe_NeitherIsLost(t *testing.T) {
	for range 20 {
		ax := newTestRuntime(t)
		s := domainRuntime.Surface{Mode: domainRuntime.SurfaceModeListen, Path: "/"}
		_, err := ax.Send(t.Context(), commands.SetSurface{Namespace: probeNs, ExecutionID: testExecutionID, Surface: s})
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Go(func() {
			drainOnly(t, ax,
				wizard.Event{Kind: wizard.EventKindStepStarted, StepIndex: 0},
				wizard.Event{Kind: wizard.EventKindPID, PID: 4242},
			)
		})
		wg.Go(func() {
			hooks := runtimeinternal.CatalogHooks{
				SurfaceReady: func(context.Context, domain.Namespace, domainRuntime.Surface) bool { return true },
			}
			runtimeinternal.ProbeSurface(t.Context(), hooks, ax, probeNs.String(), testExecutionID, s, time.Millisecond, time.Second)
		})
		wg.Wait()

		rt, err := ax.Get(t.Context(), probeNs.String())
		require.NoError(t, err)
		require.Equal(t, 4242, rt.Execution.PID)
		require.NotNil(t, rt.Execution.Surface)
		require.True(t, rt.Execution.Surface.Ready)
		require.Equal(t, domainRuntime.StepStatusRunning, rt.Execution.Steps[0].Status)
	}
}
