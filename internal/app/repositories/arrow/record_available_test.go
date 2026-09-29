package arrow_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowMocks "github.com/rabbytesoftware/quiver.core/internal/app/mocks"
	arrowRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

// versionConflict is what asynx returns when another writer appended to the
// row between this writer's read and its append.
func versionConflict() error {
	return fmt.Errorf("%w: version conflict", asynxModels.ErrPipelineFailed)
}

// rowSequence answers Get with rows in turn, repeating the last one.
func rowSequence(rows ...domain.Arrow) func(context.Context, string) (domain.Arrow, error) {
	var calls atomic.Int32
	return func(context.Context, string) (domain.Arrow, error) {
		i := int(calls.Add(1)) - 1
		if i >= len(rows) {
			i = len(rows) - 1
		}
		return rows[i], nil
	}
}

func TestCheckAvailable_VersionConflict_JudgesTheRowAgain(t *testing.T) {
	installed := domain.Resolved{Ref: "v1.0.0", Commit: "c100"}
	advanced := domain.Resolved{Ref: "v1.1.0", Commit: "c110"}
	ahead := &domain.Available{Ref: "v1.1.0", Commit: "c110"}
	row := func(resolved domain.Resolved, available *domain.Available) domain.Arrow {
		return domain.Arrow{SelectorKind: domain.SelectorChannel, Resolved: resolved, Available: available}
	}

	testCases := []struct {
		name      string
		rows      []domain.Arrow
		conflicts int
		want      *domain.Available
		wantSends int
		wantErr   error
	}{
		{
			name:      "a concurrent check recorded the same answer first",
			rows:      []domain.Arrow{row(installed, nil), row(installed, ahead)},
			conflicts: 1,
			want:      ahead,
			wantSends: 1,
		},
		{
			name:      "the row advanced to the target first",
			rows:      []domain.Arrow{row(installed, nil), row(advanced, nil)},
			conflicts: 1,
			wantSends: 1,
		},
		{
			name:      "a conflict the retry wins",
			rows:      []domain.Arrow{row(installed, nil)},
			conflicts: 1,
			want:      ahead,
			wantSends: 2,
		},
		{
			name:      "conflicts that never stop",
			rows:      []domain.Arrow{row(installed, nil)},
			conflicts: 99,
			wantSends: 3,
			wantErr:   apperrors.ErrStateViolation,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var sends atomic.Int32
			ax := &arrowMocks.AsynxArrow{
				ExistsFn: func(context.Context, string) (bool, error) { return true, nil },
				GetFn:    rowSequence(tc.rows...),
				SendWaitFn: func(context.Context, asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
					if int(sends.Add(1)) <= tc.conflicts {
						return asynxModels.Event[domain.Arrow]{}, versionConflict()
					}
					return asynxModels.Event[domain.Arrow]{}, nil
				},
			}
			cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, ax, &mocks.Vault{},
				&mocks.Manifold{SnapshotResult: stableSnapshot("v1.1.0", "c110")})

			got, err := cat.CheckAvailable(context.Background(), stableNs())

			assert.Equal(t, tc.wantSends, int(sends.Load()))
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// A passive check hands over the row as a read model saw it; by the time it
// writes, the row may have advanced. The answer must be about the row as it
// stands, or an update's advance is followed by an Available naming the very
// commit it just installed.
func TestRunVersionCheck_RowAdvancedMeanwhile_JudgesTheCurrentRow(t *testing.T) {
	ctx := context.Background()
	axArrow := newTestAsynxArrow(t)
	ns := stableNs()
	seedSelectorRow(t, axArrow, ns, domain.SelectorChannel, domain.Resolved{Ref: "v1.0.0", Commit: "c100"})
	stale, err := axArrow.Get(ctx, ns.String())
	require.NoError(t, err)
	_, err = axArrow.SendWait(ctx, arrowcmds.AdvanceArrow{
		Namespace: ns,
		Resolved:  domain.Resolved{Ref: "v1.1.0", Commit: "c110"},
	})
	require.NoError(t, err)

	target := domain.Available{Ref: "v1.1.0", Commit: "c110"}
	r := &arrowStoreMocks.MockCQRS{
		CheckDriftFn: func(_ context.Context, judged domain.Arrow) (*domain.Available, bool) {
			if judged.Resolved.Commit == target.Commit {
				return nil, true
			}
			return &target, true
		},
	}
	var synced []bool
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil,
		arrowRepo.WithVersionOutdatedSync(func(_ context.Context, _ domain.Namespace, outdated bool) error {
			synced = append(synced, outdated)
			return nil
		}))

	arrowRepo.RunVersionCheckForTest(cat, ctx, stale)

	got, err := axArrow.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.Nil(t, got.Available, "the advanced row is current")
	assert.Equal(t, []bool{false}, synced)
}

func TestRunVersionCheck_VersionConflict_RecordsOnRetry(t *testing.T) {
	var sends atomic.Int32
	var recorded atomic.Pointer[domain.Available]
	ax := &arrowMocks.AsynxArrow{
		GetFn: rowSequence(domain.Arrow{SelectorKind: domain.SelectorChannel}),
		SendWaitFn: func(_ context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			if sends.Add(1) == 1 {
				return asynxModels.Event[domain.Arrow]{}, versionConflict()
			}
			rec, ok := cmd.(arrowcmds.RecordAvailable)
			require.True(t, ok)
			recorded.Store(rec.Available)
			return asynxModels.Event[domain.Arrow]{}, nil
		},
	}
	r := &arrowStoreMocks.MockCQRS{
		CheckDriftFn: func(context.Context, domain.Arrow) (*domain.Available, bool) { return ahead(), true },
	}
	var synced []bool
	cat := arrowRepo.NewTestable(r, ax, nil, nil,
		arrowRepo.WithVersionOutdatedSync(func(_ context.Context, _ domain.Namespace, outdated bool) error {
			synced = append(synced, outdated)
			return nil
		}))

	arrowRepo.RunVersionCheckForTest(cat, context.Background(), domain.Arrow{Namespace: stableNs()})

	assert.Equal(t, int32(2), sends.Load())
	assert.Equal(t, ahead(), recorded.Load())
	assert.Equal(t, []bool{true}, synced)
}

func TestRunVersionCheck_RecordFails_StillSyncsTheAnswer(t *testing.T) {
	ax := &arrowMocks.AsynxArrow{
		GetFn: rowSequence(domain.Arrow{SelectorKind: domain.SelectorChannel}),
		SendWaitFn: func(context.Context, asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			return asynxModels.Event[domain.Arrow]{}, asynxModels.ErrValidation
		},
	}
	r := &arrowStoreMocks.MockCQRS{
		CheckDriftFn: func(context.Context, domain.Arrow) (*domain.Available, bool) { return ahead(), true },
	}
	var synced []bool
	cat := arrowRepo.NewTestable(r, ax, nil, nil,
		arrowRepo.WithVersionOutdatedSync(func(_ context.Context, _ domain.Namespace, outdated bool) error {
			synced = append(synced, outdated)
			return nil
		}))

	arrowRepo.RunVersionCheckForTest(cat, context.Background(), domain.Arrow{Namespace: stableNs()})

	assert.Equal(t, []bool{true}, synced, "the answer holds even when recording it failed")
}

func TestRunVersionCheck_RowGone_WritesNothing(t *testing.T) {
	var checked atomic.Bool
	ax := &arrowMocks.AsynxArrow{
		GetFn: func(context.Context, string) (domain.Arrow, error) {
			return domain.Arrow{}, asynxModels.ErrNotFound
		},
	}
	r := &arrowStoreMocks.MockCQRS{
		CheckDriftFn: func(context.Context, domain.Arrow) (*domain.Available, bool) {
			checked.Store(true)
			return ahead(), true
		},
	}
	var synced []bool
	cat := arrowRepo.NewTestable(r, ax, nil, nil,
		arrowRepo.WithVersionOutdatedSync(func(_ context.Context, _ domain.Namespace, outdated bool) error {
			synced = append(synced, outdated)
			return nil
		}))

	arrowRepo.RunVersionCheckForTest(cat, context.Background(), domain.Arrow{Namespace: stableNs()})

	assert.False(t, checked.Load(), "a row that is gone is not checked")
	assert.Empty(t, synced)
}
