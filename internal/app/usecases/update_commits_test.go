package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// blockedCommit is a detached commit held inside its re-resolve until
// released, so a test can act while the commit is in flight.
type blockedCommit struct {
	entered  chan struct{}
	release  chan struct{}
	aborted  chan struct{}
	log      *callLog
	usecase  *runtimeUsecase
	returned chan struct{}
}

// startBlockedCommit ends a successful update whose commit then blocks in
// its re-resolve until released or aborted.
func startBlockedCommit(t *testing.T) *blockedCommit {
	t.Helper()
	a, rt, log := commitFixture(true, nil)
	b := &blockedCommit{
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
		aborted:  make(chan struct{}),
		log:      log,
		returned: make(chan struct{}),
	}
	a.TargetUnmovedFn = func(ctx context.Context, _ domain.Namespace, target domain.Available) (bool, error) {
		close(b.entered)
		select {
		case <-b.release:
			log.add("re-resolve " + target.Commit)
			return true, nil
		case <-ctx.Done():
			close(b.aborted)
			return false, ctx.Err()
		}
	}
	b.usecase = newRuntimeUsecase(a, rt, &ucmocks.MockGraph{})
	b.usecase.detach = func(fn func()) {
		go func() {
			defer close(b.returned)
			fn()
		}()
	}
	b.usecase.targets.put(rollingRow, rollingTarget())

	b.usecase.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))
	waitClosed(t, b.entered, "the commit never started")
	return b
}

func waitClosed(t *testing.T, ch <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal(failure)
	}
}

// drainAsync starts Drain and waits until it refuses new commits, the point
// from which it is waiting for the running ones.
func drainAsync(t *testing.T, uc *runtimeUsecase, ctx context.Context) <-chan error {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- uc.Drain(ctx) }()
	require.Eventually(t, uc.commits.isDraining, 5*time.Second, time.Millisecond,
		"Drain never began refusing commits")
	return result
}

func TestRuntimeDrain_CommitInFlight_WaitsUntilItLands(t *testing.T) {
	b := startBlockedCommit(t)

	drained := drainAsync(t, b.usecase, context.Background())
	select {
	case err := <-drained:
		t.Fatalf("Drain returned (%v) while a commit was still in flight", err)
	default:
	}

	close(b.release)

	select {
	case err := <-drained:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Drain never returned after the commit landed")
	}
	assert.Equal(t, []string{"re-resolve c2", "advance c2", "reconcile badge"}, b.log.all(),
		"the commit must have landed by the time Drain returns")
}

func TestRuntimeDrain_Timeout_AbortsTheCommitAndReportsIt(t *testing.T) {
	b := startBlockedCommit(t)
	ctx, cancel := context.WithCancel(context.Background())

	drained := drainAsync(t, b.usecase, ctx)
	cancel()

	select {
	case err := <-drained:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("Drain ignored its deadline")
	}
	waitClosed(t, b.aborted, "a drain that gave up must abort the commit it abandoned")
	waitClosed(t, b.returned, "the aborted commit never returned")
	assert.Empty(t, b.log.all(), "an aborted commit stamps nothing")
}

func TestRuntimeDrain_NothingInFlight_ReturnsAtOnce(t *testing.T) {
	testCases := []struct {
		name string
		ctx  func() context.Context
	}{
		{name: "live context", ctx: context.Background},
		{name: "context already done", ctx: func() context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			uc := newUC(&ucmocks.MockArrow{}, &ucmocks.MockRuntime{}, &ucmocks.MockGraph{})

			require.NoError(t, uc.Drain(tc.ctx()))
			require.NoError(t, uc.Drain(tc.ctx()), "a second drain finds nothing left to wait for")
		})
	}
}

// Once a drain began no commit may start: its stores are about to close.
// The row stays outdated and the next update runs again.
func TestRuntimeDrain_RefusesCommitsAfterwards(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	require.NoError(t, uc.Drain(context.Background()))
	uc.targets.put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Empty(t, log.all(), "a refused commit stamps nothing")
	assert.False(t, uc.Settling(rollingRow), "a refused commit leaves nothing settling")
	_, remembered := uc.targets.take(rollingRow)
	assert.False(t, remembered, "the refused end still releases the row")
}

// A failed update commits nothing, so a drain refusing it has nothing to say.
func TestRuntimeDrain_FailedUpdateAfterwards_CommitsNothing(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	require.NoError(t, uc.Drain(context.Background()))
	uc.targets.put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeFailed))

	assert.Empty(t, log.all())
	assert.False(t, uc.Settling(rollingRow))
}

func TestRuntimeSettling_CoversTheWholeUpdateUntilItsCommitLands(t *testing.T) {
	b := startBlockedCommit(t)
	other := domain.Namespace("github.com/u/other@v1")

	assert.True(t, b.usecase.Settling(rollingRow), "a commit in flight is settling")
	assert.False(t, b.usecase.Settling(other), "only the committing row is settling")

	close(b.release)
	waitClosed(t, b.returned, "the commit never returned")

	assert.False(t, b.usecase.Settling(rollingRow), "a landed commit leaves nothing settling")
}

func TestRuntimeSettling_UpdateBegunButNotEnded(t *testing.T) {
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	uc := f.usecase()

	require.NoError(t, uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil))

	assert.True(t, uc.Settling(rollingRow), "an update whose end has not been handled is settling")
}

func TestRuntimeSettling_EndedWithoutCommit_IsNotSettling(t *testing.T) {
	testCases := []struct {
		name    string
		outcome domainRuntime.ExecutionOutcome
		record  bool
	}{
		{name: "failed update", outcome: domainRuntime.ExecutionOutcomeFailed, record: true},
		{name: "no target recorded", outcome: domainRuntime.ExecutionOutcomeSuccess},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a, rt, _ := commitFixture(true, nil)
			uc := newUC(a, rt, &ucmocks.MockGraph{})
			if tc.record {
				uc.targets.put(rollingRow, rollingTarget())
			}

			uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, tc.outcome))

			assert.False(t, uc.Settling(rollingRow))
		})
	}
}

// Reset clears a runtime stuck mid-update, so the update it abandoned must
// not keep the row settling, or block the next update, forever.
func TestRuntimeReset_ReleasesTheRememberedTarget(t *testing.T) {
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	uc := f.usecase()
	require.NoError(t, uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil))

	require.NoError(t, uc.Reset(context.Background(), rollingRow))

	assert.False(t, uc.Settling(rollingRow))
	require.NoError(t, uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil),
		"the next update is admitted")
}

func TestRuntimeReset_ForgetFails_KeepsTheRememberedTarget(t *testing.T) {
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	f.runtime.ForgetFn = func(context.Context, domain.Namespace) error { return errors.New("store down") }
	uc := f.usecase()
	require.NoError(t, uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil))

	require.Error(t, uc.Reset(context.Background(), rollingRow))

	assert.True(t, uc.Settling(rollingRow))
	err := uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestUpdateCommits_RepeatedTimeouts_AbortOnce(t *testing.T) {
	commits := newUpdateCommits()
	require.True(t, commits.begin(rollingRow))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.ErrorIs(t, commits.drain(ctx), context.Canceled)
	require.ErrorIs(t, commits.drain(ctx), context.Canceled, "a second drain that gives up must not abort twice")

	commits.done(rollingRow)
	require.NoError(t, commits.drain(context.Background()))
}

// A settle whose commit ran out of time restores the row under a context of
// its own, which a drain that gives up must still be able to abort: no write
// outlives the stores.
func TestRuntimeDrain_Abort_CancelsARestoreAfterACommitTimeout(t *testing.T) {
	a, rt, _ := commitFixture(true, nil)
	a.TargetUnmovedFn = func(ctx context.Context, _ domain.Namespace, _ domain.Available) (bool, error) {
		<-ctx.Done()
		return false, ctx.Err()
	}
	restoring := make(chan struct{})
	a.RefreshToTargetFn = func(ctx context.Context, _ domain.Namespace, _ domain.Available) (*domain.Arrow, error) {
		close(restoring)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	uc.commitTimeout = 10 * time.Millisecond
	uc.targets.put(rollingRow, rollingTarget())
	settled := make(chan struct{})
	go func() {
		uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))
		close(settled)
	}()
	waitClosed(t, restoring, "the restore never began")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, uc.Drain(ctx), context.Canceled)

	waitClosed(t, settled, "the drain's abort never reached the restore")
}

// Once a drain began, a bracket that fails after staging writes nothing
// more: the stores are about to close.
func TestRuntimeDrain_Begun_BracketRestoreIsRefused(t *testing.T) {
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	f.arrow.GetFn = func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		return &domain.Arrow{Namespace: ns, Resolved: domain.Resolved{Ref: "nightly-latest", Commit: "c1"}}, nil
	}
	uc := f.usecase()
	f.runtime.BeginUpdateFn = func(context.Context, domain.Namespace, map[string]string, string) error {
		require.NoError(t, uc.Drain(context.Background()))
		return errors.New("refused")
	}

	require.Error(t, uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil))

	assert.NotContains(t, f.log.all(), "refresh to c1")
}

// A check held while the row settled may record a newer Available after the
// settle's own reconcile read the row: the settle re-derives the badge again
// before it lets the row go, and checks after that sync it themselves.
func TestRuntimeOnUpdateEnded_CheckHeldDuringTheReconcile_ReconcilesAgain(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	uc := newUC(a, rt, &ucmocks.MockGraph{})
	calls := 0
	rt.ReconcileVersionBadgeFn = func(context.Context, domain.Namespace) error {
		calls++
		log.add("reconcile badge")
		if calls == 1 {
			assert.True(t, uc.HoldBadge(rollingRow), "a check landing now is held")
		}
		return nil
	}
	uc.targets.put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance c2", "reconcile badge", "reconcile badge"}, log.all())
	assert.False(t, uc.HoldBadge(rollingRow), "once settled, a check syncs the badge itself")
}
