package mocks_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func TestDiscoveryService_Start(t *testing.T) {
	m := &mocks.DiscoveryService{
		StartResult: usecases.Job{ID: "job-1"},
		StartErr:    errTest,
	}

	job, err := m.Start(ctx, "chrom")

	assert.Equal(t, errTest, err)
	assert.Equal(t, "job-1", job.ID)
	assert.Equal(t, 1, m.StartCalls())
	assert.Equal(t, "chrom", m.StartQuery())
}

func TestDiscoveryService_Get(t *testing.T) {
	want := &usecases.Job{ID: "job-1", Status: usecases.JobCompleted}
	m := &mocks.DiscoveryService{GetResult: want, GetErr: errTest}

	got, err := m.Get(ctx, "job-1")

	assert.Equal(t, errTest, err)
	assert.Equal(t, want, got)
	assert.Equal(t, "job-1", m.GetID())
}

func TestDiscoveryService_Cancel(t *testing.T) {
	m := &mocks.DiscoveryService{}

	m.Cancel(ctx, "job-1")
	m.Cancel(ctx, "job-2")

	assert.Equal(t, []string{"job-1", "job-2"}, m.Cancelled())
}

func TestDiscoveryService_OnResultAndEmit(t *testing.T) {
	m := &mocks.DiscoveryService{}

	got := make([]usecases.StreamItem, 0, 2)
	m.OnResult(func(item usecases.StreamItem) { got = append(got, item) })
	m.OnResult(func(item usecases.StreamItem) { got = append(got, item) })
	require.Equal(t, 2, m.Listeners())

	m.Emit(usecases.StreamItem{JobID: "job-1"})

	require.Len(t, got, 2)
	assert.Equal(t, "job-1", got[0].JobID)
	assert.Equal(t, "job-1", got[1].JobID)
}

func TestDiscoveryService_EmitWithoutListeners(t *testing.T) {
	m := &mocks.DiscoveryService{}

	assert.NotPanics(t, func() { m.Emit(usecases.StreamItem{JobID: "job-1"}) })
	assert.Zero(t, m.Listeners())
}
