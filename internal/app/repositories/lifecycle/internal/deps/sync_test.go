package deps

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestSyncTargetDeps_LandsTheDiffThenSyncsWhatIsPending(t *testing.T) {
	boom := errors.New("boom")
	added := domain.Namespace("github.com/user/new@v1")
	removed := domain.Namespace("github.com/user/old@v1")

	testCases := []struct {
		name       string
		diff       models.DepDiff
		markErr    error
		state      domain.ArrowState
		stateErr   error
		wantMarked bool
		wantSynced bool
		wantErr    error
	}{
		{name: "no change on a ready row syncs nothing", state: domain.ArrowStateReady},
		{
			name:       "a changed target lands pending and syncs it",
			diff:       models.DepDiff{Added: []domain.DependencyEdge{{Namespace: added}}, Removed: []domain.DependencyEdge{{Namespace: removed}}},
			state:      domain.ArrowStateOutdated,
			wantMarked: true,
			wantSynced: true,
		},
		{name: "an outdated row syncs what an earlier attempt left", state: domain.ArrowStateOutdated, wantSynced: true},
		{
			name:       "a change that cannot be recorded fails",
			diff:       models.DepDiff{Added: []domain.DependencyEdge{{Namespace: added}}},
			markErr:    boom,
			wantMarked: true,
			wantErr:    boom,
		},
		{name: "a state that cannot be read fails", stateErr: boom, wantErr: boom},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var marked *domainRuntime.DepSyncInfo
			synced := false
			rt := &mocks.MockRuntime{
				MarkOutdatedFn: func(_ context.Context, _ domain.Namespace, a, r []domain.Namespace) error {
					marked = &domainRuntime.DepSyncInfo{AddedDeps: a, RemovedDeps: r}
					return tc.markErr
				},
				GetStateFn: func(context.Context, domain.Namespace) (domain.ArrowState, error) {
					return tc.state, tc.stateErr
				},
				GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
					synced = true
					return &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateOutdated}, nil
				},
			}
			g := &mocks.MockGraph{DiffDepsFn: func(_, _ *domain.Arrow) models.DepDiff { return tc.diff }}
			uc := newUC(&mocks.MockArrow{}, rt, g)

			err := uc.deps.SyncTargetDeps(context.Background(), rollingRow, &domain.Arrow{}, &domain.Arrow{})

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantMarked, marked != nil)
			if tc.wantMarked && tc.markErr == nil {
				assert.Equal(t, []domain.Namespace{added}, marked.AddedDeps)
				assert.Equal(t, []domain.Namespace{removed}, marked.RemovedDeps)
			}
			assert.Equal(t, tc.wantSynced, synced)
		})
	}
}

func TestRuntimeSyncDeps_AddedDepIsItsOwnRow_IsRefused(t *testing.T) {
	rt := &mocks.MockRuntime{
		GetRuntimeFn: func(_ context.Context, ns domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
			return &domainRuntime.ArrowRuntime{
				Ref:            ns,
				State:          domain.ArrowStateOutdated,
				PendingDepSync: &domainRuntime.DepSyncInfo{AddedDeps: []domain.Namespace{rollingRow.BareNamespace()}},
			}, nil
		},
	}
	a := &mocks.MockArrow{
		AddDependencyFn: func(context.Context, domain.Namespace) (domain.Namespace, error) { return rollingRow, nil },
	}

	err := newUC(a, rt, &mocks.MockGraph{}).syncDeps(context.Background(), rollingRow)

	require.ErrorIs(t, err, apperrors.ErrInvalidManifest)
}
