package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func emitting(
	namespaces ...string,
) *stubPipeline {
	return &stubPipeline{fn: func(
		_ context.Context,
		_ string,
		emit func(discovery.Result),
	) (discovery.Outcome, error) {
		for _, ns := range namespaces {
			emit(discovery.Result{Namespace: domain.Namespace(ns)})
		}
		return discovery.Outcome{}, nil
	}}
}

func blocking(
	entered chan<- struct{},
	cancelled chan<- struct{},
) *stubPipeline {
	return &stubPipeline{fn: func(
		ctx context.Context,
		_ string,
		_ func(discovery.Result),
	) (discovery.Outcome, error) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		return discovery.Outcome{}, nil
	}}
}

func TestDiscovery_Replay_ReturnsEmittedItemsInOrderWithSeq(t *testing.T) {
	uc := NewDiscoveryUsecase(emitting("github.com/u/a", "github.com/u/b"))

	job, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	waitCompleted(t, uc, job.ID)

	items := uc.Replay(job.ID)
	require.Len(t, items, 2)
	assert.Equal(t, domain.Namespace("github.com/u/a"), items[0].Result.Namespace)
	assert.Equal(t, uint64(1), items[0].Seq)
	assert.Equal(t, uint64(2), items[1].Seq)
	assert.Equal(t, job.ID, items[1].JobID)
}

func TestDiscovery_Replay_UnknownJobIsEmpty(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{})

	assert.Empty(t, uc.Replay("missing"))
}

func TestDiscovery_Done_ClosesWhenPassFinishes(t *testing.T) {
	uc := NewDiscoveryUsecase(emitting("github.com/u/a"))

	job, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)

	select {
	case <-uc.Done(job.ID):
	case <-time.After(2 * time.Second):
		t.Fatal("done never closed")
	}
	assert.Len(t, uc.Replay(job.ID), 1, "every item is buffered before done closes")
}

func TestDiscovery_Done_UnknownJobIsAlreadyClosed(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{})

	select {
	case <-uc.Done("missing"):
	default:
		t.Fatal("an unknown job must not block its subscriber")
	}
}

func TestDiscovery_Detach_CancelsAfterGraceWhenLastSubscriberLeaves(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	uc := NewDiscoveryUsecase(blocking(entered, cancelled), WithDiscoverySubscriberGrace(10*time.Millisecond))

	job, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	<-entered

	uc.Attach(job.ID)
	uc.Attach(job.ID)
	uc.Detach(job.ID)
	select {
	case <-cancelled:
		t.Fatal("cancelled while a subscriber remained")
	case <-time.After(50 * time.Millisecond):
	}

	uc.Detach(job.ID)
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("pass was not cancelled after the last subscriber left")
	}
}

func TestDiscovery_Attach_WithinGraceKeepsThePassRunning(t *testing.T) {
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	uc := NewDiscoveryUsecase(blocking(entered, cancelled), WithDiscoverySubscriberGrace(80*time.Millisecond))

	job, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	<-entered

	uc.Attach(job.ID)
	uc.Detach(job.ID)
	uc.Attach(job.ID)

	select {
	case <-cancelled:
		t.Fatal("a reconnect inside the grace must not cancel the pass")
	case <-time.After(200 * time.Millisecond):
	}

	uc.Cancel(context.Background(), job.ID)
	<-cancelled
}

func TestDiscovery_AttachDetach_UnknownJobAndUnderflowAreNoOps(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{})

	uc.Attach("missing")
	uc.Detach("missing")

	job, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	waitCompleted(t, uc, job.ID)
	uc.Detach(job.ID)
	uc.Attach(job.ID)
	uc.Detach(job.ID)
}

func TestDiscovery_Detach_FinishedPassArmsNoTimer(t *testing.T) {
	uc := NewDiscoveryUsecase(&stubPipeline{}, WithDiscoverySubscriberGrace(time.Millisecond))

	job, err := uc.Start(context.Background(), "chrom")
	require.NoError(t, err)
	waitCompleted(t, uc, job.ID)

	uc.Attach(job.ID)
	uc.Detach(job.ID)

	impl := uc.(*discoveryUsecase)
	impl.mu.Lock()
	defer impl.mu.Unlock()
	assert.Nil(t, impl.sessions[job.ID].idle)
}
