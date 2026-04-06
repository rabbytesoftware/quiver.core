package stepreporter_test

import (
	"context"
	"testing"

	"github.com/char2cs/asynx"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/stepreporter"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildAsynxRuntime(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	return ax
}

func seedRuntime(t *testing.T, ax asynx.Asynx[domainRuntime.ArrowRuntime], ns domain.Namespace, stepCount int) {
	t.Helper()
	steps := make([]domainRuntime.StepProgress, stepCount)
	for i := 0; i < stepCount; i++ {
		steps[i] = domainRuntime.StepProgress{Index: i, Status: domainRuntime.StepStatusPending}
	}
	_, err := ax.Send(context.Background(), mocks.RuntimeCmdWithExecution{
		NS:    ns,
		State: domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{
			Method: "_execute",
			Steps:  steps,
		},
	})
	require.NoError(t, err)
	ax.WaitPublish()
}

func TestStepReporter_New_TwoParams(t *testing.T) {
	ax := buildAsynxRuntime(t)
	// New takes exactly 2 params — compile-time check
	r := stepreporter.New(ax, domain.Namespace("github.com/org/arrow"))
	assert.NotNil(t, r)
}

func TestStepReporter_OnStepStarted_SendsRunningAtExactIndex(t *testing.T) {
	ax := buildAsynxRuntime(t)
	ns := domain.Namespace("github.com/test/arrow1")
	seedRuntime(t, ax, ns, 3)

	r := stepreporter.New(ax, ns)
	r.OnStepStarted(2)
	ax.WaitPublish()

	rt, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, rt.ActiveRun)
	assert.Equal(t, domainRuntime.StepStatusRunning, rt.ActiveRun.Steps[2].Status)
}

func TestStepReporter_OnStepCompleted_SendsCompletedAtExactIndex(t *testing.T) {
	ax := buildAsynxRuntime(t)
	ns := domain.Namespace("github.com/test/arrow1")
	seedRuntime(t, ax, ns, 3)

	r := stepreporter.New(ax, ns)
	r.OnStepCompleted(1)
	ax.WaitPublish()

	rt, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domainRuntime.StepStatusCompleted, rt.ActiveRun.Steps[1].Status)
}

func TestStepReporter_OnStepFailed_SendsFailedWithErrorAtExactIndex(t *testing.T) {
	ax := buildAsynxRuntime(t)
	ns := domain.Namespace("github.com/test/arrow1")
	seedRuntime(t, ax, ns, 2)

	r := stepreporter.New(ax, ns)
	r.OnStepFailed(0, assert.AnError)
	ax.WaitPublish()

	rt, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domainRuntime.StepStatusFailed, rt.ActiveRun.Steps[0].Status)
	require.NotNil(t, rt.ActiveRun.Steps[0].Error)
}
