package runtime_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	runtimeMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func newReadyRuntime(t *testing.T) (runtime.Runtime, domain.Namespace) {
	t.Helper()
	f := catToFuncs(&runtimeMocks.MockArrow{})
	lc, err := runtime.NewTestable(newTestAsynxRuntime(t), nil, successAssembler(), f.markInstalled, f.markUninstalled, f.markLastUsed, f.hasDependents, f.listArrows, func(context.Context) ([]domain.Namespace, error) { return nil, nil })
	require.NoError(t, err)
	ns := testNs()
	require.NoError(t, lc.MarkReady(context.Background(), ns))
	return lc, ns
}

func stagedBinary() domainRuntime.PendingActivation {
	return domainRuntime.PendingActivation{
		Version:  "nightly-2",
		StagedAt: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
		Path:     "/vault/quiver-new",
		Size:     10,
		Digest:   "abc",
	}
}

func TestPendingActivation_StageMarkClear(t *testing.T) {
	lc, ns := newReadyRuntime(t)
	ctx := context.Background()

	require.NoError(t, lc.RecordPendingActivation(ctx, ns, stagedBinary()))
	rt, err := lc.GetRuntime(ctx, ns)
	require.NoError(t, err)
	require.NotNil(t, rt.PendingActivation)
	assert.Equal(t, "nightly-2", rt.PendingActivation.Version)
	assert.False(t, rt.PendingActivation.Activating)

	require.NoError(t, lc.MarkActivating(ctx, ns))
	rt, err = lc.GetRuntime(ctx, ns)
	require.NoError(t, err)
	assert.True(t, rt.PendingActivation.Activating)

	require.NoError(t, lc.ClearPendingActivation(ctx, ns))
	rt, err = lc.GetRuntime(ctx, ns)
	require.NoError(t, err)
	assert.Nil(t, rt.PendingActivation)
}

func TestPendingActivation_StateViolations(t *testing.T) {
	lc, ns := newReadyRuntime(t)
	ctx := context.Background()

	assert.ErrorIs(t, lc.MarkActivating(ctx, ns), apperrors.ErrStateViolation, "nothing staged")
	assert.ErrorIs(t, lc.ClearPendingActivation(ctx, ns), apperrors.ErrStateViolation, "nothing staged")

	empty := stagedBinary()
	empty.Path = ""
	assert.ErrorIs(t, lc.RecordPendingActivation(ctx, ns, empty), apperrors.ErrStateViolation)

	assert.Error(t, lc.RecordPendingActivation(ctx, domain.Namespace("github.com/never/seen@v1"), stagedBinary()))
}

func TestPendingActivation_BroadcastsStageAndClear(t *testing.T) {
	lc, ns := newReadyRuntime(t)
	ctx := context.Background()
	var mu sync.Mutex
	var staged, cleared []domainRuntime.ArrowRuntime
	require.NoError(t, lc.OnRuntimeActivationStaged(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		mu.Lock()
		defer mu.Unlock()
		staged = append(staged, rt)
	}))
	require.NoError(t, lc.OnRuntimeActivationCleared(func(_ context.Context, rt domainRuntime.ArrowRuntime) {
		mu.Lock()
		defer mu.Unlock()
		cleared = append(cleared, rt)
	}))

	require.NoError(t, lc.RecordPendingActivation(ctx, ns, stagedBinary()))
	require.NoError(t, lc.ClearPendingActivation(ctx, ns))

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(staged) == 1 && len(cleared) == 1
	}, 5*time.Second, 10*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	assert.NotNil(t, staged[0].PendingActivation)
	assert.Nil(t, cleared[0].PendingActivation)
}
