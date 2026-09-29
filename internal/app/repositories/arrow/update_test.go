package arrow_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	arrowMocks "github.com/rabbytesoftware/quiver.core/internal/app/mocks"
	arrowRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func stableNs() domain.Namespace {
	return domain.Namespace("github.com/user/repo@stable")
}

func stableSnapshot(latestTag, commit string) domain.RefSnapshot {
	return domain.RefSnapshot{
		Tags:     map[string]string{"v1.0.0": "c100", latestTag: commit},
		Branches: map[string]string{"main": "cmain"},
		Head:     "main",
	}
}

// ─── CheckAvailable ──────────────────────────────────────────────────────────

func TestCheckAvailable_RecordsWhatALiveSnapshotFinds(t *testing.T) {
	installed := domain.Resolved{Ref: "v1.0.0", Commit: "c100", Fingerprint: "c100"}
	ahead := &domain.Available{Ref: "v1.1.0", Commit: "c110"}

	testCases := []struct {
		name         string
		snap         domain.RefSnapshot
		recorded     *domain.Available
		want         *domain.Available
		wantOutdated bool
	}{
		{
			name:         "a newer member is recorded as available",
			snap:         stableSnapshot("v1.1.0", "c110"),
			want:         ahead,
			wantOutdated: true,
		},
		{
			name:     "a current row clears what an older check recorded",
			snap:     stableSnapshot("v1.0.0", "c100"),
			recorded: ahead,
		},
		{
			name:         "an unchanged answer is returned without a new event",
			snap:         stableSnapshot("v1.1.0", "c110"),
			recorded:     ahead,
			want:         ahead,
			wantOutdated: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			axArrow := newTestAsynxArrow(t)
			ns := stableNs()
			seedSelectorRow(t, axArrow, ns, domain.SelectorChannel, installed)
			if tc.recorded != nil {
				_, err := axArrow.SendWait(ctx, arrowcmds.RecordAvailable{Namespace: ns, Available: tc.recorded})
				require.NoError(t, err)
			}
			m := &mocks.Manifold{SnapshotResult: tc.snap}
			var synced []bool
			cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, &mocks.Vault{}, m,
				arrowRepo.WithVersionOutdatedSync(func(_ context.Context, _ domain.Namespace, outdated bool) error {
					synced = append(synced, outdated)
					return nil
				}))

			got, err := cat.CheckAvailable(ctx, ns)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			row, err := axArrow.Get(ctx, ns.String())
			require.NoError(t, err)
			assert.Equal(t, tc.want, row.Available)
			assert.Equal(t, installed, row.Resolved)
			assert.Equal(t, []bool{tc.wantOutdated}, synced)
			assert.Equal(t, 1, m.FreshSnapshotCalls, "the check must not answer from a cached snapshot")
			assert.Zero(t, m.SnapshotCalls)
		})
	}
}

func TestCheckAvailable_Failures(t *testing.T) {
	remoteDown := errors.New("remote down")
	installed := domain.Resolved{Ref: "v1.0.0", Commit: "c100"}

	testCases := []struct {
		name    string
		seed    bool
		snap    domain.RefSnapshot
		snapErr error
		getErr  error
		wantErr error
	}{
		{name: "no row", wantErr: apperrors.ErrNotFound},
		{name: "snapshot fails", seed: true, snapErr: remoteDown, wantErr: apperrors.ErrFetchFailed},
		{
			name:    "the channel vanished",
			seed:    true,
			snap:    domain.RefSnapshot{Branches: map[string]string{"main": "c"}, Head: "main", Tags: map[string]string{"nightly": "n"}},
			wantErr: apperrors.ErrNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			axArrow := newTestAsynxArrow(t)
			if tc.seed {
				seedSelectorRow(t, axArrow, stableNs(), domain.SelectorChannel, installed)
			}
			var synced atomic.Int32
			cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, &mocks.Vault{},
				&mocks.Manifold{SnapshotResult: tc.snap, SnapshotErr: tc.snapErr},
				arrowRepo.WithVersionOutdatedSync(func(context.Context, domain.Namespace, bool) error {
					synced.Add(1)
					return nil
				}))

			_, err := cat.CheckAvailable(ctx, stableNs())

			require.ErrorIs(t, err, tc.wantErr)
			assert.Zero(t, synced.Load(), "a failed check answers nothing")
			if tc.seed {
				row, getErr := axArrow.Get(ctx, stableNs().String())
				require.NoError(t, getErr)
				assert.Nil(t, row.Available)
			}
		})
	}
}

func TestCheckAvailable_AsynxFailures(t *testing.T) {
	boom := errors.New("event store down")

	testCases := []struct {
		name    string
		ax      *arrowMocks.AsynxArrow
		wantErr error
	}{
		{
			name:    "exists fails",
			ax:      &arrowMocks.AsynxArrow{ExistsFn: func(context.Context, string) (bool, error) { return false, boom }},
			wantErr: boom,
		},
		{
			name: "get fails",
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(context.Context, string) (bool, error) { return true, nil },
				GetFn:    func(context.Context, string) (domain.Arrow, error) { return domain.Arrow{}, boom },
			},
			wantErr: boom,
		},
		{
			name: "record is rejected",
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(context.Context, string) (bool, error) { return true, nil },
				GetFn: func(context.Context, string) (domain.Arrow, error) {
					return domain.Arrow{SelectorKind: domain.SelectorChannel}, nil
				},
				SendWaitFn: func(context.Context, asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
					return asynxModels.Event[domain.Arrow]{}, asynxModels.ErrValidation
				},
			},
			wantErr: apperrors.ErrStateViolation,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, tc.ax, &mocks.Vault{},
				&mocks.Manifold{SnapshotResult: stableSnapshot("v1.1.0", "c110")})

			_, err := cat.CheckAvailable(context.Background(), stableNs())

			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// ─── TargetUnmoved ───────────────────────────────────────────────────────────

func TestTargetUnmoved(t *testing.T) {
	target := domain.Available{Ref: "nightly-latest", Commit: "c2"}

	testCases := []struct {
		name string
		snap domain.RefSnapshot
		want bool
	}{
		{"still at the target commit", domain.RefSnapshot{Tags: map[string]string{"nightly-latest": "c2"}}, true},
		{"moved again", domain.RefSnapshot{Tags: map[string]string{"nightly-latest": "c3"}}, false},
		{"deleted", domain.RefSnapshot{Tags: map[string]string{"v1.0.0": "c1"}}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mocks.Manifold{SnapshotResult: tc.snap}
			cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), &mocks.Vault{}, m)

			got, err := cat.TargetUnmoved(context.Background(), rollingNs(), target)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, 1, m.FreshSnapshotCalls)
			assert.Zero(t, m.SnapshotCalls)
		})
	}
}

func TestTargetUnmoved_SnapshotFails(t *testing.T) {
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), &mocks.Vault{},
		&mocks.Manifold{SnapshotErr: errors.New("remote down")})

	_, err := cat.TargetUnmoved(context.Background(), rollingNs(), domain.Available{Ref: "x", Commit: "c"})

	require.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

// ─── RefreshToTarget ─────────────────────────────────────────────────────────

func TestRefreshToTarget_StagesTheTargetManifestOnTheSameRow(t *testing.T) {
	ctx := context.Background()
	ns := rollingNs()
	axArrow := newTestAsynxArrow(t)

	var mu sync.Mutex
	var projected []domain.Arrow
	r := &arrowStoreMocks.MockCQRS{
		ProjectFn: func(_ context.Context, a domain.Arrow) error {
			mu.Lock()
			defer mu.Unlock()
			projected = append(projected, a)
			return nil
		},
	}
	v := &mocks.Vault{}
	var fetchedCommit string
	m := &mocks.Manifold{
		ResolveArrowAtCommitFn: func(_ context.Context, got domain.Namespace, commit string) (*domain.Arrow, []byte, string, error) {
			fetchedCommit = commit
			return &domain.Arrow{Namespace: got, ArrowMeta: domain.ArrowMeta{Name: "Target"}}, []byte("raw"), "arrow.yaml", nil
		},
	}
	hub := &recordingHub{}
	cat, err := arrowRepo.NewTestableProjecting(r, axArrow, v, m, hub)
	require.NoError(t, err)
	var updated atomic.Int32
	require.NoError(t, cat.OnArrowUpdated(func(context.Context, domain.Namespace, *domain.Arrow) error {
		updated.Add(1)
		return nil
	}))

	installed := domain.Resolved{Ref: "nightly-latest", Commit: "c1", Fingerprint: "c1"}
	target := domain.Available{Ref: "nightly-latest", Commit: "c2"}
	seedSelectorRow(t, axArrow, ns, domain.SelectorChannel, installed)
	_, err = axArrow.SendWait(ctx, arrowcmds.RecordAvailable{Namespace: ns, Available: &target})
	require.NoError(t, err)

	staged, err := cat.RefreshToTarget(ctx, ns, target)

	require.NoError(t, err)
	assert.Equal(t, "Target", staged.Name)
	assert.Equal(t, ns, staged.Namespace)
	assert.Equal(t, "c2", fetchedCommit)

	row, err := axArrow.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.Equal(t, "Target", row.Name)
	assert.Equal(t, installed, row.Resolved, "nothing is installed until the update commits")
	assert.Equal(t, &target, row.Available)

	assert.Equal(t, []string{"delete " + ns.String(), "put " + ns.String()}, v.ArrowOps)
	require.NotEmpty(t, v.PutArrowFiles)
	assert.Equal(t, "arrow.yaml", v.PutArrowFiles[len(v.PutArrowFiles)-1].Filename)

	assert.Equal(t, int32(1), updated.Load(), "a refreshed manifest re-syncs the dependency graph")
	mu.Lock()
	last := projected[len(projected)-1]
	mu.Unlock()
	assert.Equal(t, "Target", last.Name, "the read model carries the staged manifest")
	assert.Equal(t, installed, last.Resolved)
	assert.Equal(t, apphub.CatalogUpserted, hub.kinds()[len(hub.kinds())-1])
}

func TestRefreshToTarget_Failures(t *testing.T) {
	fetchErr := errors.New("clone failed")
	target := domain.Available{Ref: "nightly-latest", Commit: "c2"}

	testCases := []struct {
		name     string
		seed     bool
		target   domain.Available
		vault    *mocks.Vault
		fetchErr error
		wantErr  error
	}{
		{name: "target without a commit", seed: true, target: domain.Available{Ref: "x"}, vault: &mocks.Vault{}, wantErr: apperrors.ErrInvalidNamespace},
		{name: "no row", target: target, vault: &mocks.Vault{}, wantErr: apperrors.ErrNotFound},
		{name: "fetch fails", seed: true, target: target, vault: &mocks.Vault{}, fetchErr: fetchErr, wantErr: apperrors.ErrFetchFailed},
		{name: "cache cannot be replaced", seed: true, target: target, vault: &mocks.Vault{PutArrowErr: errors.New("disk full")}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			axArrow := newTestAsynxArrow(t)
			installed := domain.Resolved{Ref: "nightly-latest", Commit: "c1"}
			if tc.seed {
				seedSelectorRow(t, axArrow, rollingNs(), domain.SelectorChannel, installed)
			}
			m := &mocks.Manifold{
				ResolveArrowAtCommitResult: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Target"}},
				ResolveArrowAtCommitErr:    tc.fetchErr,
			}
			cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, axArrow, tc.vault, m)

			_, err := cat.RefreshToTarget(ctx, rollingNs(), tc.target)

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
			if tc.seed {
				row, getErr := axArrow.Get(ctx, rollingNs().String())
				require.NoError(t, getErr)
				assert.Equal(t, "Old", row.Name, "a failed refresh leaves the stored manifest alone")
			}
		})
	}
}

func TestRefreshToTarget_AsynxFailures(t *testing.T) {
	boom := errors.New("event store down")
	target := domain.Available{Ref: "nightly-latest", Commit: "c2"}

	testCases := []struct {
		name    string
		ax      *arrowMocks.AsynxArrow
		wantErr error
	}{
		{
			name:    "exists fails",
			ax:      &arrowMocks.AsynxArrow{ExistsFn: func(context.Context, string) (bool, error) { return false, boom }},
			wantErr: boom,
		},
		{
			name: "refresh is rejected",
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(context.Context, string) (bool, error) { return true, nil },
				SendWaitFn: func(context.Context, asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
					return asynxModels.Event[domain.Arrow]{}, asynxModels.ErrPipelineFailed
				},
			},
			wantErr: apperrors.ErrStateViolation,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mocks.Manifold{ResolveArrowAtCommitResult: &domain.Arrow{}}
			cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, tc.ax, &mocks.Vault{}, m)

			_, err := cat.RefreshToTarget(context.Background(), rollingNs(), target)

			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// ─── AddDependency ───────────────────────────────────────────────────────────

func TestAddDependency_AddsTheIdentityTheSelectorResolvesTo(t *testing.T) {
	ctx := context.Background()
	axArrow := newTestAsynxArrow(t)
	identity := stableNs()
	resolved := domain.Resolved{Ref: "v1.1.0", Commit: "c110", Fingerprint: "c110"}
	var asked domain.Namespace
	r := &arrowStoreMocks.MockCQRS{
		ResolveInstallFn: func(_ context.Context, ns domain.Namespace) (domain.Namespace, *domain.Arrow, error) {
			asked = ns
			return identity, &domain.Arrow{
				Namespace:     identity,
				ArrowMeta:     domain.ArrowMeta{Name: "Dep"},
				UserInstalled: true,
				SelectorKind:  domain.SelectorChannel,
				Resolved:      resolved,
			}, nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, &mocks.Vault{}, &mocks.Manifold{})

	got, err := cat.AddDependency(ctx, identity.BareNamespace())

	require.NoError(t, err)
	assert.Equal(t, identity, got)
	assert.Equal(t, identity.BareNamespace(), asked)
	row, err := axArrow.Get(ctx, identity.String())
	require.NoError(t, err)
	assert.False(t, row.UserInstalled, "a dependency is never user-installed")
	assert.Equal(t, domain.SelectorChannel, row.SelectorKind)
	assert.Equal(t, resolved, row.Resolved)
	assert.Equal(t, "Dep", row.Name)
}

// Another dependent reaching the same identity must not rewrite the row, and
// above all must not mark it user-installed.
func TestAddDependency_ExistingIdentityIsLeftAlone(t *testing.T) {
	ctx := context.Background()
	axArrow := newTestAsynxArrow(t)
	identity := stableNs()
	_, err := axArrow.SendWait(ctx, arrowcmds.AddArrow{
		Namespace:    identity,
		ArrowMeta:    domain.ArrowMeta{Name: "Installed"},
		SelectorKind: domain.SelectorChannel,
		Resolved:     domain.Resolved{Ref: "v1.0.0", Commit: "c100"},
	})
	require.NoError(t, err)
	r := &arrowStoreMocks.MockCQRS{
		ResolveInstallFn: func(context.Context, domain.Namespace) (domain.Namespace, *domain.Arrow, error) {
			return identity, &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Newer"}}, nil
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, &mocks.Vault{}, &mocks.Manifold{})

	got, err := cat.AddDependency(ctx, identity.BareNamespace())

	require.NoError(t, err)
	assert.Equal(t, identity, got)
	row, err := axArrow.Get(ctx, identity.String())
	require.NoError(t, err)
	assert.Equal(t, "Installed", row.Name)
	assert.False(t, row.UserInstalled)
}

func TestAddDependency_Failures(t *testing.T) {
	boom := errors.New("event store down")
	resolveOK := func(context.Context, domain.Namespace) (domain.Namespace, *domain.Arrow, error) {
		return stableNs(), &domain.Arrow{}, nil
	}

	testCases := []struct {
		name    string
		resolve func(context.Context, domain.Namespace) (domain.Namespace, *domain.Arrow, error)
		ax      *arrowMocks.AsynxArrow
		wantErr error
	}{
		{
			name: "selector does not resolve",
			resolve: func(context.Context, domain.Namespace) (domain.Namespace, *domain.Arrow, error) {
				return "", nil, apperrors.ErrInvalidNamespace
			},
			ax:      &arrowMocks.AsynxArrow{},
			wantErr: apperrors.ErrInvalidNamespace,
		},
		{
			name:    "exists fails",
			resolve: resolveOK,
			ax:      &arrowMocks.AsynxArrow{ExistsFn: func(context.Context, string) (bool, error) { return false, boom }},
			wantErr: boom,
		},
		{
			name:    "add fails",
			resolve: resolveOK,
			ax: &arrowMocks.AsynxArrow{
				ExistsFn: func(context.Context, string) (bool, error) { return false, nil },
				SendWaitFn: func(context.Context, asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
					return asynxModels.Event[domain.Arrow]{}, boom
				},
			},
			wantErr: boom,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{ResolveInstallFn: tc.resolve}, tc.ax,
				&mocks.Vault{}, &mocks.Manifold{})

			_, err := cat.AddDependency(context.Background(), stableNs().BareNamespace())

			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// Two installs racing to add the same dependency: the loser's rejected add
// means the row exists, which is all it wanted.
func TestAddDependency_LostRaceIsNotAnError(t *testing.T) {
	cat := arrowRepo.NewTestable(
		&arrowStoreMocks.MockCQRS{ResolveInstallFn: func(context.Context, domain.Namespace) (domain.Namespace, *domain.Arrow, error) {
			return stableNs(), &domain.Arrow{}, nil
		}},
		&arrowMocks.AsynxArrow{
			ExistsFn: func(context.Context, string) (bool, error) { return false, nil },
			SendWaitFn: func(context.Context, asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
				return asynxModels.Event[domain.Arrow]{}, asynxModels.ErrValidation
			},
		},
		&mocks.Vault{}, &mocks.Manifold{},
	)

	got, err := cat.AddDependency(context.Background(), stableNs().BareNamespace())

	require.NoError(t, err)
	assert.Equal(t, stableNs(), got)
}
