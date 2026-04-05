package stepreporter_test

import (
	"context"
	"fmt"
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

func seedRuntime(t *testing.T, ax asynx.Asynx[domainRuntime.ArrowRuntime], ns domain.Namespace) {
	t.Helper()
	require.NoError(t, ax.Send(context.Background(), mocks.RuntimeCmdWithExecution{
		NS:    ns,
		State: domain.ArrowStateRunning,
		ActiveRun: &domainRuntime.RunRecord{
			Method: "_execute",
			Steps: []domainRuntime.StepProgress{
				{Index: 0, Status: domainRuntime.StepStatusPending},
				{Index: 1, Status: domainRuntime.StepStatusPending},
			},
		},
	}))
	ax.WaitPublish()
}

func TestStepReporter_OnStepStarted_SendsRunningStatus(t *testing.T) {
	ax := buildAsynxRuntime(t)
	ns := domain.Namespace("github.com/test/arrow1")
	seedRuntime(t, ax, ns)

	r := stepreporter.New(ax, ns, 0)
	r.OnStepStarted(0)
	ax.WaitPublish()

	rt, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, rt.ActiveRun)
	assert.Equal(t, domainRuntime.StepStatusRunning, rt.ActiveRun.Steps[0].Status)
}

func TestStepReporter_OnStepCompleted_SendsCompletedStatus(t *testing.T) {
	ax := buildAsynxRuntime(t)
	ns := domain.Namespace("github.com/test/arrow1")
	seedRuntime(t, ax, ns)

	r := stepreporter.New(ax, ns, 0)
	r.OnStepCompleted(1)
	ax.WaitPublish()

	rt, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domainRuntime.StepStatusCompleted, rt.ActiveRun.Steps[1].Status)
}

func TestStepReporter_OnStepFailed_SendsFailedStatusWithError(t *testing.T) {
	ax := buildAsynxRuntime(t)
	ns := domain.Namespace("github.com/test/arrow1")
	seedRuntime(t, ax, ns)

	r := stepreporter.New(ax, ns, 0)
	r.OnStepFailed(0, fmt.Errorf("something broke"))
	ax.WaitPublish()

	rt, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	require.NotNil(t, rt.ActiveRun.Steps[0].Error)
	assert.Equal(t, "something broke", *rt.ActiveRun.Steps[0].Error)
	assert.Equal(t, domainRuntime.StepStatusFailed, rt.ActiveRun.Steps[0].Status)
}

func TestStepReporter_IndexOffset_ShiftsStepIndex(t *testing.T) {
	ax := buildAsynxRuntime(t)
	ns := domain.Namespace("github.com/test/arrow1")
	seedRuntime(t, ax, ns)

	r := stepreporter.New(ax, ns, 1) // offset 1: step 0 from wizard → index 1 in aggregate
	r.OnStepCompleted(0)
	ax.WaitPublish()

	rt, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domainRuntime.StepStatusCompleted, rt.ActiveRun.Steps[1].Status)
	assert.Equal(t, domainRuntime.StepStatusPending, rt.ActiveRun.Steps[0].Status)
}
