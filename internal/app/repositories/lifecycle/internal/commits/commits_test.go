package commits

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const rollingRow = domain.Namespace("github.com/char2cs/crowbar@nightly-latest")

func TestUpdateCommits_RepeatedTimeouts_AbortOnce(t *testing.T) {
	c := New().(*commits)
	require.True(t, c.Begin(rollingRow))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, c.drain(ctx), context.Canceled)
	require.ErrorIs(t, c.drain(ctx), context.Canceled, "a second drain that gives up must not abort twice")

	c.Done(rollingRow)
	require.NoError(t, c.drain(context.Background()))
}

func TestCommits_InFlight_FollowsBeginAndDone(t *testing.T) {
	c := New()

	require.True(t, c.Begin(rollingRow))
	require.True(t, c.Begin(rollingRow))
	assert.True(t, c.InFlight(rollingRow))

	c.Done(rollingRow)
	assert.True(t, c.InFlight(rollingRow), "one of two commits is still running")
	c.Done(rollingRow)
	assert.False(t, c.InFlight(rollingRow))
}

func TestCommits_Drain_WaitsForTheRunningCommit(t *testing.T) {
	c := New()
	require.True(t, c.Begin(rollingRow))

	drained := make(chan error, 1)
	go func() { drained <- c.Drain(context.Background()) }()
	require.Eventually(t, c.IsDraining, 5*time.Second, time.Millisecond)
	assert.False(t, c.Begin(rollingRow), "a draining tracker refuses new commits")

	c.Done(rollingRow)

	select {
	case err := <-drained:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Drain never returned after the commit was done")
	}
}

func TestCommits_Drain_GivingUpAbortsBoundCommits(t *testing.T) {
	c := New()
	require.True(t, c.Begin(rollingRow))
	commitCtx, cancelCommit := c.Bound(context.Background(), time.Hour)
	defer cancelCommit()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Drain(ctx)

	require.ErrorIs(t, err, context.Canceled)
	select {
	case <-commitCtx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("a drain that gave up never aborted the commit")
	}
}

func TestCommits_Bound_EndsAfterItsTimeout(t *testing.T) {
	c := New()

	ctx, cancel := c.Bound(context.Background(), time.Millisecond)
	defer cancel()

	select {
	case <-ctx.Done():
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("a bound commit outlived its timeout")
	}
}
