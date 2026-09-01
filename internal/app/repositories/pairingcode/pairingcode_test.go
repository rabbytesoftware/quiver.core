package pairingcode_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/pairingcode"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func newTestAsynx(t *testing.T) asynx.Asynx[auth.PairingCode] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[auth.PairingCode]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 2, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	return ax
}

func TestNew_NilAsynx_ReturnsError(t *testing.T) {
	_, err := pairingcode.New(nil)
	assert.Error(t, err)
}

func TestGenerate_ReturnsPendingCode(t *testing.T) {
	repo, err := pairingcode.New(newTestAsynx(t))
	require.NoError(t, err)

	got, err := repo.Generate(context.Background(), 5*time.Minute)
	require.NoError(t, err)

	assert.Len(t, got.Code, 6)
	assert.Equal(t, auth.PairingCodeStatePending, got.State)
	assert.True(t, got.ExpiresAt.After(got.CreatedAt))
}

func TestGenerate_ExhaustsRetriesOnRepeatedCollision(t *testing.T) {
	calls := 0
	repo, err := pairingcode.New(newTestAsynx(t), pairingcode.WithCodeGenerator(func() (string, error) {
		calls++
		return "111111", nil
	}))
	require.NoError(t, err)

	ctx := context.Background()

	// First call creates the pending "111111" code.
	_, err = repo.Generate(ctx, time.Minute)
	require.NoError(t, err)

	// The generator always returns the same, now-taken code, so every retry
	// attempt in this second call collides and the loop must exhaust itself
	// rather than succeed by luck.
	_, err = repo.Generate(ctx, time.Minute)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
	assert.Equal(t, 6, calls) // 1 success + 5 exhausted retries
}

func TestGenerate_CodeGeneratorFails_ReturnsError(t *testing.T) {
	repo, err := pairingcode.New(newTestAsynx(t), pairingcode.WithCodeGenerator(func() (string, error) {
		return "", errors.New("entropy source unavailable")
	}))
	require.NoError(t, err)

	_, err = repo.Generate(context.Background(), time.Minute)
	assert.Error(t, err)
}

func TestClaim_ValidCode_Succeeds(t *testing.T) {
	repo, err := pairingcode.New(newTestAsynx(t))
	require.NoError(t, err)

	ctx := context.Background()
	code, err := repo.Generate(ctx, time.Minute)
	require.NoError(t, err)

	err = repo.Claim(ctx, code.Code, "dev-1", "laptop")
	assert.NoError(t, err)
}

func TestClaim_UnknownCode_ReturnsInvalidPairingCode(t *testing.T) {
	repo, err := pairingcode.New(newTestAsynx(t))
	require.NoError(t, err)

	err = repo.Claim(context.Background(), "000000", "dev-1", "laptop")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidPairingCode)
}

func TestClaim_AlreadyClaimedCode_ReturnsInvalidPairingCode(t *testing.T) {
	repo, err := pairingcode.New(newTestAsynx(t))
	require.NoError(t, err)

	ctx := context.Background()
	code, err := repo.Generate(ctx, time.Minute)
	require.NoError(t, err)
	require.NoError(t, repo.Claim(ctx, code.Code, "dev-1", "laptop"))

	err = repo.Claim(ctx, code.Code, "dev-2", "phone")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidPairingCode)
}

func TestClaim_ExpiredCode_ReturnsInvalidPairingCode(t *testing.T) {
	repo, err := pairingcode.New(newTestAsynx(t))
	require.NoError(t, err)

	ctx := context.Background()
	code, err := repo.Generate(ctx, time.Nanosecond)
	require.NoError(t, err)

	time.Sleep(time.Millisecond)

	err = repo.Claim(ctx, code.Code, "dev-1", "laptop")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidPairingCode)
}

func TestShutdown_DrainsAsynx(t *testing.T) {
	repo, err := pairingcode.New(newTestAsynx(t))
	require.NoError(t, err)

	assert.NoError(t, repo.Shutdown(context.Background()))
}
