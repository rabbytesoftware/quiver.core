package runtime_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	runtimeMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func TestRuntime_ClearVersionBadge_OutdatedWithoutDepSync_BecomesReady(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateOutdated)
	repo := newRepoWithAssembler(t, ax, successAssembler())

	require.NoError(t, repo.ClearVersionBadge(context.Background(), ns))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateReady, got.State)
}

func TestRuntime_ClearVersionBadge_PendingDepSync_StaysOutdated(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateReady)
	repo := newRepoWithAssembler(t, ax, successAssembler())
	require.NoError(t, repo.MarkOutdated(context.Background(), ns, []domain.Namespace{"a/b/c@v1"}, nil))
	ax.WaitPublish()

	require.NoError(t, repo.ClearVersionBadge(context.Background(), ns))

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, domain.ArrowStateOutdated, got.State)
	require.NotNil(t, got.PendingDepSync)
}

func TestRuntime_ClearVersionBadge_NoOpStates(t *testing.T) {
	testCases := []struct {
		name  string
		seed  bool
		state domain.ArrowState
	}{
		{name: "ready stays ready", seed: true, state: domain.ArrowStateReady},
		{name: "running is not a badge", seed: true, state: domain.ArrowStateRunning},
		{name: "no aggregate grows none", seed: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := newTestAsynxRuntime(t)
			ns := testNs()
			if tc.seed {
				seedState(t, ax, ns, tc.state)
			}
			cleared := countTopic(t, ax, "runtime.outdated_cleared.*")
			repo := newRepoWithAssembler(t, ax, successAssembler())

			require.NoError(t, repo.ClearVersionBadge(context.Background(), ns))
			ax.WaitPublish()

			assert.Zero(t, cleared.Load())
			exists, err := ax.Exists(context.Background(), ns.String())
			require.NoError(t, err)
			assert.Equal(t, tc.seed, exists)
		})
	}
}

func TestRuntime_ClearVersionBadge_ClosedStore_ReturnsError(t *testing.T) {
	ax := newTestAsynxRuntime(t)
	ns := testNs()
	seedState(t, ax, ns, domain.ArrowStateOutdated)
	repo := newRepoWithAssembler(t, ax, successAssembler())
	require.NoError(t, ax.Shutdown(context.Background()))

	assert.Error(t, repo.ClearVersionBadge(context.Background(), ns))
}

// An update's end leaves the badge to the update bracket, which re-derives it
// once its commit landed; every other method's end re-derives it at once.
func TestRuntime_ExecutionEnd_ReconcilesTheBadgeExceptAfterAnUpdate(t *testing.T) {
	testCases := []struct {
		name  string
		begin func(runtime.Runtime, domain.Namespace) error
		want  int32
	}{
		{name: "execute", begin: func(r runtime.Runtime, ns domain.Namespace) error {
			return r.BeginExecution(context.Background(), ns, domain.MethodExecute, nil)
		}, want: 1},
		{name: "update", begin: func(r runtime.Runtime, ns domain.Namespace) error {
			return r.BeginUpdate(context.Background(), ns, nil, "v1.3.0")
		}, want: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := newTestAsynxRuntime(t)
			ns := testNs()
			seedReadyRuntime(t, ax, ns)
			var reconciled atomic.Int32
			w := &mocks.Wizard{StartFn: func(context.Context, wizardPkg.RunRequest) wizardPkg.Execution {
				return mocks.NewDoneExecution(domainRuntime.ExecutionOutcomeSuccess)
			}}
			f := catToFuncs(&runtimeMocks.MockArrow{})
			repo, err := runtime.NewTestable(ax, w, successAssembler(), f.markInstalled, f.markUninstalled, f.markLastUsed,
				f.hasDependents, f.listArrows, func(context.Context) ([]domain.Namespace, error) { return nil, nil },
				func(context.Context, domain.Namespace) error {
					reconciled.Add(1)
					return nil
				})
			require.NoError(t, err)
			ended, unsub, err := repo.ListenEnded(context.Background(), ns)
			require.NoError(t, err)
			defer unsub()

			require.NoError(t, tc.begin(repo, ns))
			select {
			case <-ended:
			case <-time.After(5 * time.Second):
				require.FailNow(t, "the execution never ended")
			}
			require.NoError(t, repo.Shutdown(context.Background()))

			assert.Equal(t, tc.want, reconciled.Load())
		})
	}
}

func TestRuntime_ReconcileVersionBadge_RunsTheCatalogReconcile(t *testing.T) {
	boom := errors.New("catalog closed")
	testCases := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "reconciled", err: nil},
		{name: "reconcile fails", err: boom, wantErr: boom},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ax := newTestAsynxRuntime(t)
			var got domain.Namespace
			f := catToFuncs(&runtimeMocks.MockArrow{})
			repo, err := runtime.NewTestable(ax, nil, successAssembler(), f.markInstalled, f.markUninstalled, f.markLastUsed,
				f.hasDependents, f.listArrows, func(context.Context) ([]domain.Namespace, error) { return nil, nil },
				func(_ context.Context, ns domain.Namespace) error {
					got = ns
					return tc.err
				})
			require.NoError(t, err)

			err = repo.ReconcileVersionBadge(context.Background(), testNs())

			assert.Equal(t, testNs(), got)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
