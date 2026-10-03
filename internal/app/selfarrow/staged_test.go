package selfarrow_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/selfarrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type recordingTrigger struct{ fired []string }

func (r *recordingTrigger) Fire(path string) { r.fired = append(r.fired, path) }

// stagedFixture is a daemon booting with a binary staged by an earlier run.
type stagedFixture struct {
	t       *testing.T
	ns      domain.Namespace
	path    string
	pending *domainRuntime.PendingActivation
	calls   []string
	rt      *mocks.MockRuntime
	trig    *recordingTrigger
	getErr  error
	markErr error
	clrErr  error
}

func newStagedFixture(t *testing.T, version string) *stagedFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "quiver-new")
	require.NoError(t, os.WriteFile(path, []byte("staged build"), 0o755))
	size, digest, err := selfupdate.Fingerprint(context.Background(), path)
	require.NoError(t, err)

	f := &stagedFixture{
		t:    t,
		ns:   selfNs().WithRef("nightly-latest"),
		path: path,
		pending: &domainRuntime.PendingActivation{
			Version: version, Commit: "c2", Path: path, Size: size, Digest: digest,
		},
		trig: &recordingTrigger{},
	}
	f.rt = &mocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			if f.getErr != nil {
				return nil, f.getErr
			}
			if f.pending == nil {
				return &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady}, nil
			}
			return &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady, PendingActivation: f.pending}, nil
		},
		MarkActivatingFn: func(context.Context, domain.Namespace) error {
			f.calls = append(f.calls, "mark")
			return f.markErr
		},
		ClearPendingActivationFn: func(context.Context, domain.Namespace) error {
			f.calls = append(f.calls, "clear")
			return f.clrErr
		},
	}
	return f
}

func (f *stagedFixture) reconcile(running selfarrow.Running) (selfarrow.StagedOutcome, error) {
	f.t.Helper()
	return selfarrow.ReconcileStaged(context.Background(), f.rt, f.trigger(), f.ns, running)
}

func (f *stagedFixture) trigger() selfarrow.Handover {
	return f.trig
}

func (f *stagedFixture) fileGone() bool {
	_, err := os.Stat(f.path)
	return errors.Is(err, os.ErrNotExist)
}

func oldBuild() selfarrow.Running {
	return selfarrow.Running{Version: "nightly-1", Commit: "c1", Executable: "/usr/local/bin/quiver"}
}

func TestReconcileStaged_NothingStaged(t *testing.T) {
	f := newStagedFixture(t, "nightly-2")
	f.pending = nil

	outcome, err := f.reconcile(oldBuild())

	require.NoError(t, err)
	assert.Equal(t, selfarrow.StagedNone, outcome)
	assert.Empty(t, f.calls)
	assert.Empty(t, f.trig.fired)
}

func TestReconcileStaged_NoRuntimeAtAll(t *testing.T) {
	f := newStagedFixture(t, "nightly-2")
	f.rt.GetRuntimeFn = func(context.Context, domain.Namespace) (*domainRuntime.ArrowRuntime, error) { return nil, nil }

	outcome, err := f.reconcile(oldBuild())

	require.NoError(t, err)
	assert.Equal(t, selfarrow.StagedNone, outcome)
}

func TestReconcileStaged_ANewerVerifiedBinaryIsHandedOverToOnce(t *testing.T) {
	f := newStagedFixture(t, "nightly-2")

	outcome, err := f.reconcile(oldBuild())

	require.NoError(t, err)
	assert.Equal(t, selfarrow.StagedHandedOver, outcome)
	assert.Equal(t, []string{"mark"}, f.calls, "the attempt is recorded before the handover, so a failure is never retried at the next boot")
	assert.Equal(t, []string{f.path}, f.trig.fired)
	assert.False(t, f.fileGone())
}

func TestReconcileStaged_NoTrigger_LeavesTheBinaryStaged(t *testing.T) {
	f := newStagedFixture(t, "nightly-2")

	outcome, err := selfarrow.ReconcileStaged(context.Background(), f.rt, nil, f.ns, oldBuild())

	require.NoError(t, err)
	assert.Equal(t, selfarrow.StagedKept, outcome)
	assert.Empty(t, f.calls)
	assert.False(t, f.fileGone())
}

func TestReconcileStaged_ARollingReleaseWithANewCommitIsNewer(t *testing.T) {
	f := newStagedFixture(t, "nightly-latest")
	running := selfarrow.Running{Version: "nightly-latest", Commit: "c1", Executable: "/usr/local/bin/quiver"}

	outcome, err := f.reconcile(running)

	require.NoError(t, err)
	assert.Equal(t, selfarrow.StagedHandedOver, outcome)
}

func TestReconcileStaged_ABinaryThatIsAlreadyRunningIsApplied(t *testing.T) {
	testCases := []struct {
		name    string
		running func(f *stagedFixture) selfarrow.Running
		marked  bool
	}{
		{name: "same version and commit", running: func(*stagedFixture) selfarrow.Running {
			return selfarrow.Running{Version: "nightly-2", Commit: "c2", Executable: "/usr/local/bin/quiver"}
		}},
		{name: "running from the staged file", running: func(f *stagedFixture) selfarrow.Running {
			return selfarrow.Running{Version: "whatever", Commit: "other", Executable: f.path}
		}},
		{name: "after a handover that worked", marked: true, running: func(f *stagedFixture) selfarrow.Running {
			return selfarrow.Running{Version: "nightly-2", Commit: "c2", Executable: f.path}
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newStagedFixture(t, "nightly-2")
			f.pending.Activating = tc.marked

			outcome, err := f.reconcile(tc.running(f))

			require.NoError(t, err)
			assert.Equal(t, selfarrow.StagedApplied, outcome)
			assert.Equal(t, []string{"clear"}, f.calls)
			assert.Empty(t, f.trig.fired)
			assert.False(t, f.fileGone(), "the file may be the one running")
		})
	}
}

func TestReconcileStaged_AHandoverThatFailedIsDiscardedNotRetried(t *testing.T) {
	f := newStagedFixture(t, "nightly-2")
	f.pending.Activating = true

	outcome, err := f.reconcile(oldBuild())

	require.NoError(t, err)
	assert.Equal(t, selfarrow.StagedHandoverFailed, outcome)
	assert.Equal(t, []string{"clear"}, f.calls)
	assert.Empty(t, f.trig.fired)
	assert.True(t, f.fileGone(), "the stale binary of the failed handover is removed")
}

func TestReconcileStaged_AnUnusableBinaryIsDiscardedNotExecuted(t *testing.T) {
	testCases := []struct {
		name   string
		damage func(f *stagedFixture)
	}{
		{name: "truncated", damage: func(f *stagedFixture) { require.NoError(t, os.WriteFile(f.path, []byte("staged"), 0o755)) }},
		{name: "replaced", damage: func(f *stagedFixture) { require.NoError(t, os.WriteFile(f.path, []byte("STAGED BUILD"), 0o755)) }},
		{name: "gone", damage: func(f *stagedFixture) { require.NoError(t, os.Remove(f.path)) }},
		{name: "never fingerprinted", damage: func(f *stagedFixture) { f.pending.Digest = "" }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newStagedFixture(t, "nightly-2")
			tc.damage(f)

			outcome, err := f.reconcile(oldBuild())

			require.NoError(t, err)
			assert.Equal(t, selfarrow.StagedDiscarded, outcome)
			assert.Equal(t, []string{"clear"}, f.calls)
			assert.Empty(t, f.trig.fired)
			assert.True(t, f.fileGone())
		})
	}
}

func TestReconcileStaged_AnOlderBinaryIsDiscarded(t *testing.T) {
	testCases := []struct {
		name    string
		staged  string
		running string
		want    selfarrow.StagedOutcome
	}{
		{name: "lower build number", staged: "beta-26.5-4", running: "beta-26.5-5", want: selfarrow.StagedDiscarded},
		{name: "lower release", staged: "stable-26.5.1", running: "stable-26.5.2", want: selfarrow.StagedDiscarded},
		{name: "numeric not textual", staged: "stable-26.9.0", running: "stable-26.10.0", want: selfarrow.StagedDiscarded},
		{name: "higher build number", staged: "beta-26.5-6", running: "beta-26.5-5", want: selfarrow.StagedHandedOver},
		{name: "longer core", staged: "v1.2.1", running: "v1.2", want: selfarrow.StagedHandedOver},
		{name: "no numbers to compare", staged: "nightly-latest", running: "dev-build", want: selfarrow.StagedHandedOver},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newStagedFixture(t, tc.staged)

			outcome, err := f.reconcile(selfarrow.Running{Version: tc.running, Commit: "c1", Executable: "/usr/local/bin/quiver"})

			require.NoError(t, err)
			assert.Equal(t, tc.want, outcome)
		})
	}
}

func TestReconcileStaged_Errors(t *testing.T) {
	boom := errors.New("event store down")

	t.Run("runtime unreadable", func(t *testing.T) {
		f := newStagedFixture(t, "nightly-2")
		f.getErr = boom

		_, err := f.reconcile(oldBuild())

		require.ErrorIs(t, err, boom)
	})

	t.Run("mark fails", func(t *testing.T) {
		f := newStagedFixture(t, "nightly-2")
		f.markErr = boom

		outcome, err := f.reconcile(oldBuild())

		require.ErrorIs(t, err, boom)
		assert.Equal(t, selfarrow.StagedNone, outcome)
		assert.Empty(t, f.trig.fired, "an attempt that could not be recorded is never made")
	})

	t.Run("somebody else marked it first", func(t *testing.T) {
		f := newStagedFixture(t, "nightly-2")
		f.markErr = apperrors.ErrStateViolation

		outcome, err := f.reconcile(oldBuild())

		require.NoError(t, err)
		assert.Equal(t, selfarrow.StagedNone, outcome)
		assert.Empty(t, f.trig.fired)
	})

	t.Run("clearing a discarded record fails", func(t *testing.T) {
		f := newStagedFixture(t, "nightly-2")
		f.pending.Activating = true
		f.clrErr = boom

		_, err := f.reconcile(oldBuild())

		require.ErrorIs(t, err, boom)
	})
}
