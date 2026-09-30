package settle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestRuntimeOnUpdateEnded_TargetUnchanged_AdvancesThenReconcilesTheBadge(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	uc := newUC(a, rt, &mocks.MockGraph{})
	uc.targets.Put(rollingRow, rollingTarget())

	uc.onRuntimeEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance c2", "reconcile badge"}, log.all())
}

func TestRuntimeOnUpdateEnded_StampsNothing(t *testing.T) {
	testCases := []struct {
		name       string
		rt         domainRuntime.ArrowRuntime
		record     bool
		unmoved    bool
		unmovedErr error
		wantLog    []string
	}{
		{
			name:    "target moved during the update",
			rt:      updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess),
			record:  true,
			wantLog: []string{"re-resolve c2", "restore c1", "reconcile badge"},
		},
		{
			name:       "target cannot be re-resolved",
			rt:         updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess),
			record:     true,
			unmovedErr: errors.New("remote down"),
			wantLog:    []string{"re-resolve c2", "restore c1", "reconcile badge"},
		},
		{
			name:    "update steps failed",
			rt:      updateEnded(rollingRow, domainRuntime.ExecutionOutcomeFailed),
			record:  true,
			unmoved: true,
			wantLog: []string{"restore c1", "reconcile badge"},
		},
		{
			name:    "no return recorded",
			rt:      domainRuntime.ArrowRuntime{Ref: rollingRow},
			record:  true,
			unmoved: true,
			wantLog: []string{"restore c1", "reconcile badge"},
		},
		{
			name:    "no update began toward a target",
			rt:      updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess),
			unmoved: true,
			wantLog: []string{"reconcile badge"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a, rt, log := commitFixture(tc.unmoved, tc.unmovedErr)
			uc := newUC(a, rt, &mocks.MockGraph{})
			if tc.record {
				uc.targets.Put(rollingRow, rollingTarget())
			}

			uc.onUpdateEnded(context.Background(), tc.rt)

			assert.Equal(t, tc.wantLog, log.all())
		})
	}
}

// Every end releases the row's remembered target, whatever its outcome, so
// the guard against an unhandled end can never wedge the row: a failed
// update is followed by an admitted one.
func TestRuntimeOnUpdateEnded_FailedUpdateReleasesTheRow(t *testing.T) {
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	uc := f.usecase()
	require.NoError(t, uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil))

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeFailed))

	_, remembered := uc.targets.Take(rollingRow)
	assert.False(t, remembered)
	require.NoError(t, uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil))
	assert.Equal(t, []string{
		"check available", "refresh to c2", "begin update",
		"check available", "refresh to c2", "begin update",
	}, f.log.all())
}

// A version check during the update may record a newer target on the row;
// the commit stamps the target the update actually ran.
func TestRuntimeOnUpdateEnded_CommitsTheTargetTheUpdateRan(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	a.GetFn = func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		return &domain.Arrow{Namespace: ns, Available: &domain.Available{Ref: "nightly-latest", Commit: "c3"}}, nil
	}
	uc := newUC(a, rt, &mocks.MockGraph{})
	uc.targets.Put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance c2", "reconcile badge"}, log.all())
}

func TestRuntimeOnUpdateEnded_AdvanceFails_BadgeStays(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	a.AdvanceFn = func(context.Context, domain.Namespace, domain.Available) error {
		log.add("advance failed")
		return errors.New("fetch failed")
	}
	uc := newUC(a, rt, &mocks.MockGraph{})
	uc.targets.Put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance failed", "restore c1", "reconcile badge"}, log.all())
}

// A failed update leaves the row reporting what it had installed, and the
// manifest on the row must be that release's again: an install or execution
// that follows must never run the failed target's steps for the installed
// release's ${REF}.
func TestRuntimeOnUpdateEnded_NothingStamped_RestoresTheInstalledManifest(t *testing.T) {
	testCases := []struct {
		name    string
		row     *domain.Arrow
		getErr  error
		restore error
		wantLog []string
	}{
		{
			name:    "installed release restaged",
			row:     &domain.Arrow{Resolved: domain.Resolved{Ref: "v1.2.0", Commit: "c120"}},
			wantLog: []string{"restore v1.2.0@c120"},
		},
		{
			name:    "a row with no recorded commit keeps the staged manifest",
			row:     &domain.Arrow{Resolved: domain.Resolved{Ref: "v1.2.0"}},
			wantLog: nil,
		},
		{
			name:    "a row that cannot be read restores nothing",
			getErr:  errors.New("event store down"),
			wantLog: nil,
		},
		{
			name:    "a restore that fails is only logged",
			row:     &domain.Arrow{Resolved: domain.Resolved{Ref: "v1.2.0", Commit: "c120"}},
			restore: errors.New("fetch failed"),
			wantLog: []string{"restore v1.2.0@c120"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			log := &callLog{}
			a := &mocks.MockArrow{
				GetFn: func(context.Context, domain.Namespace) (*domain.Arrow, error) { return tc.row, tc.getErr },
				RefreshToTargetFn: func(_ context.Context, _ domain.Namespace, target domain.Available) (*domain.Arrow, error) {
					log.add("restore " + target.Ref + "@" + target.Commit)
					return nil, tc.restore
				},
			}
			uc := newUC(a, &mocks.MockRuntime{}, &mocks.MockGraph{})
			uc.targets.Put(rollingRow, rollingTarget())

			uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeFailed))

			assert.Equal(t, tc.wantLog, log.all())
		})
	}
}

func TestRuntimeOnUpdateEnded_ReconcileBadgeFails_IsOnlyLogged(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	rt.ReconcileVersionBadgeFn = func(context.Context, domain.Namespace) error {
		log.add("reconcile badge failed")
		return errors.New("event store down")
	}
	uc := newUC(a, rt, &mocks.MockGraph{})
	uc.targets.Put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance c2", "reconcile badge failed"}, log.all())
}

// A failed update of quiver.core's own row leaves the running build in
// charge, so its manifest is put back like any other row's; a succeeded one
// is left to the relaunched build.
func TestRuntimeOnUpdateEnded_SelfNamespace_RestoresOnlyAfterFailure(t *testing.T) {
	self, _ := metadata.GetSelfNamespaces()
	selfRow := self.WithRef("stable")

	testCases := []struct {
		name    string
		outcome domainRuntime.ExecutionOutcome
		want    []string
	}{
		{name: "failed", outcome: domainRuntime.ExecutionOutcomeFailed, want: []string{"restore c1", "reconcile badge"}},
		{name: "succeeded", outcome: domainRuntime.ExecutionOutcomeSuccess, want: []string{"reconcile badge"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			a, rt, log := commitFixture(true, nil)
			uc := newUC(a, rt, &mocks.MockGraph{})

			uc.onUpdateEnded(context.Background(), updateEnded(selfRow, tc.outcome))

			assert.Equal(t, tc.want, log.all())
		})
	}
}

// The commit leaves the runtime aggregate's ordered delivery before it
// clears that aggregate's badge, which would otherwise wait for itself: the
// handler must return before the commit does anything.
func TestRuntimeOnUpdateEnded_CommitsOffTheDeliveringGoroutine(t *testing.T) {
	a, rt, _ := commitFixture(true, nil)
	handlerReturned := make(chan struct{})
	a.TargetUnmovedFn = func(context.Context, domain.Namespace, domain.Available) (bool, error) {
		<-handlerReturned
		return true, nil
	}
	cleared := make(chan struct{})
	rt.ReconcileVersionBadgeFn = func(context.Context, domain.Namespace) error {
		close(cleared)
		return nil
	}
	uc := newDetachedUC(a, rt, &mocks.MockGraph{})
	uc.targets.Put(rollingRow, rollingTarget())

	returned := make(chan struct{})
	go func() {
		uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler waited for the commit")
	}
	close(handlerReturned)

	select {
	case <-cleared:
	case <-time.After(5 * time.Second):
		t.Fatal("the detached commit never reconciled the badge")
	}
}

// A commit whose remote hangs gives up instead of holding its goroutine
// forever.
func TestRuntimeOnUpdateEnded_CommitHasADeadline(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	a.TargetUnmovedFn = func(ctx context.Context, _ domain.Namespace, _ domain.Available) (bool, error) {
		<-ctx.Done()
		return false, ctx.Err()
	}
	uc := newUC(a, rt, &mocks.MockGraph{})
	uc.settler.commitTimeout = 20 * time.Millisecond
	uc.targets.Put(rollingRow, rollingTarget())

	done := make(chan struct{})
	go func() {
		uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the commit never gave up on a hung remote")
	}
	assert.Equal(t, []string{"restore c1", "reconcile badge"}, log.all(),
		"nothing is stamped when the re-check times out, but the row is restored and its badge follows it")
}

// blockedCommit is a detached commit held inside its re-resolve until
// released, so a test can act while the commit is in flight.
type blockedCommit struct {
	entered  chan struct{}
	release  chan struct{}
	aborted  chan struct{}
	log      *callLog
	usecase  *harness
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
	b.usecase = newDetachedUC(a, rt, &mocks.MockGraph{})
	b.usecase.detach = func(fn func()) {
		go func() {
			defer close(b.returned)
			fn()
		}()
	}
	b.usecase.targets.Put(rollingRow, rollingTarget())

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
func drainAsync(t *testing.T, uc *harness, ctx context.Context) <-chan error {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- uc.Drain(ctx) }()
	require.Eventually(t, uc.commits.IsDraining, 5*time.Second, time.Millisecond,
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
			uc := newUC(&mocks.MockArrow{}, &mocks.MockRuntime{}, &mocks.MockGraph{})

			require.NoError(t, uc.Drain(tc.ctx()))
			require.NoError(t, uc.Drain(tc.ctx()), "a second drain finds nothing left to wait for")
		})
	}
}

// Once a drain began no commit may start: its stores are about to close.
// The row stays outdated and the next update runs again.
func TestRuntimeDrain_RefusesCommitsAfterwards(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	uc := newUC(a, rt, &mocks.MockGraph{})
	require.NoError(t, uc.Drain(context.Background()))
	uc.targets.Put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Empty(t, log.all(), "a refused commit stamps nothing")
	assert.False(t, uc.Settling(rollingRow), "a refused commit leaves nothing settling")
	_, remembered := uc.targets.Take(rollingRow)
	assert.False(t, remembered, "the refused end still releases the row")
}

// A failed update commits nothing, so a drain refusing it has nothing to say.
func TestRuntimeDrain_FailedUpdateAfterwards_CommitsNothing(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	uc := newUC(a, rt, &mocks.MockGraph{})
	require.NoError(t, uc.Drain(context.Background()))
	uc.targets.Put(rollingRow, rollingTarget())

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
			uc := newUC(a, rt, &mocks.MockGraph{})
			if tc.record {
				uc.targets.Put(rollingRow, rollingTarget())
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
	uc := newUC(a, rt, &mocks.MockGraph{})
	uc.settler.commitTimeout = 10 * time.Millisecond
	uc.targets.Put(rollingRow, rollingTarget())
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
	uc := newUC(a, rt, &mocks.MockGraph{})
	calls := 0
	rt.ReconcileVersionBadgeFn = func(context.Context, domain.Namespace) error {
		calls++
		log.add("reconcile badge")
		if calls == 1 {
			assert.True(t, uc.HoldBadge(rollingRow), "a check landing now is held")
		}
		return nil
	}
	uc.targets.Put(rollingRow, rollingTarget())

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance c2", "reconcile badge", "reconcile badge"}, log.all())
	assert.False(t, uc.HoldBadge(rollingRow), "once settled, a check syncs the badge itself")
}

// A check held while the update ran is covered by the settle's own
// reconcile, which reads the row after it: the row is released after one
// reconcile, so a client that sees Ready can update again at once.
func TestRuntimeOnUpdateEnded_CheckHeldDuringTheRun_ReleasesAfterOneReconcile(t *testing.T) {
	a, rt, log := commitFixture(true, nil)
	uc := newUC(a, rt, &mocks.MockGraph{})
	settlingAtReconcile := []bool{}
	rt.ReconcileVersionBadgeFn = func(context.Context, domain.Namespace) error {
		log.add("reconcile badge")
		settlingAtReconcile = append(settlingAtReconcile, uc.Settling(rollingRow))
		return nil
	}
	uc.targets.Put(rollingRow, rollingTarget())
	require.True(t, uc.HoldBadge(rollingRow), "a detail read's check while the update runs is held")

	uc.onUpdateEnded(context.Background(), updateEnded(rollingRow, domainRuntime.ExecutionOutcomeSuccess))

	assert.Equal(t, []string{"re-resolve c2", "advance c2", "reconcile badge"}, log.all())
	assert.Equal(t, []bool{true}, settlingAtReconcile)
	assert.False(t, uc.Settling(rollingRow), "released right after its one reconcile")
}

// A bracket's own restore settles the row like an update's end: a check held
// while it ran gets the badge re-derived, and nothing stays held.
func TestRuntimeExecute_Update_RestoreAfterFailure_ReconcilesTheBadge(t *testing.T) {
	target := rollingTarget()
	f := newBracketFixture(domain.ArrowStateReady, &target)
	f.arrow.GetFn = func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		return &domain.Arrow{Namespace: ns, Resolved: domain.Resolved{Ref: "nightly-latest", Commit: "c1"}}, nil
	}
	uc := f.usecase()
	refresh := f.arrow.RefreshToTargetFn
	f.arrow.RefreshToTargetFn = func(ctx context.Context, ns domain.Namespace, a domain.Available) (*domain.Arrow, error) {
		if a.Commit == "c1" {
			assert.True(t, uc.HoldBadge(ns), "a check landing during the restore is held")
		}
		return refresh(ctx, ns, a)
	}
	f.runtime.BeginUpdateFn = func(context.Context, domain.Namespace, map[string]string, string) error {
		return errors.New("refused")
	}
	reconciled := 0
	f.runtime.ReconcileVersionBadgeFn = func(context.Context, domain.Namespace) error {
		reconciled++
		return nil
	}

	require.Error(t, uc.Execute(context.Background(), rollingRow, domain.MethodUpdate, nil))

	assert.Equal(t, 1, reconciled, "the held check's badge is re-derived")
	assert.False(t, uc.HoldBadge(rollingRow), "nothing stays held once the restore settled")
}
