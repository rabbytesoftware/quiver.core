package arrow_test

import (
	"context"
	"errors"
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
	runtimeRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
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
		GetFn: func(context.Context, string) (domain.Arrow, error) {
			return domain.Arrow{SelectorKind: domain.SelectorChannel, Available: recorded.Load()}, nil
		},
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

// Every attempt finds the row moved under it: the answer that never landed is
// not what the badge shows; the row as it stands is.
func TestRunVersionCheck_RetriesExhausted_SyncsFromTheRow(t *testing.T) {
	var sends atomic.Int32
	ax := &arrowMocks.AsynxArrow{
		GetFn: rowSequence(domain.Arrow{SelectorKind: domain.SelectorChannel}),
		SendWaitFn: func(context.Context, asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
			sends.Add(1)
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

	assert.Equal(t, int32(3), sends.Load())
	assert.Equal(t, []bool{false}, synced, "the row holds no Available, so the badge must not say outdated")
}

// The usual state before an update: the row already records the target as
// available. A passive check judging the row before the update's advance
// lands answers that same target, so it writes nothing; the badge must still
// follow the advanced row, not that answer.
func TestRunVersionCheck_AdvanceLandsWhileJudgingAnUnchangedAnswer_LeavesNoBadge(t *testing.T) {
	ctx := context.Background()
	axArrow := newTestAsynxArrow(t)
	ns := stableNs()
	installed := domain.Resolved{Ref: "v1.0.0", Commit: "c100"}
	target := domain.Available{Ref: "v1.1.0", Commit: "c110"}
	seedSelectorRow(t, axArrow, ns, domain.SelectorChannel, installed)
	_, err := axArrow.SendWait(ctx, arrowcmds.RecordAvailable{Namespace: ns, Available: &target, JudgedResolved: installed})
	require.NoError(t, err)
	stale, err := axArrow.Get(ctx, ns.String())
	require.NoError(t, err)

	var advanced atomic.Bool
	r := &arrowStoreMocks.MockCQRS{
		CheckDriftFn: func(context.Context, domain.Arrow) (*domain.Available, bool) {
			if !advanced.Swap(true) {
				_, sendErr := axArrow.SendWait(ctx, arrowcmds.AdvanceArrow{
					Namespace: ns,
					Resolved:  domain.Resolved{Ref: target.Ref, Commit: target.Commit},
				})
				require.NoError(t, sendErr)
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
	assert.Nil(t, got.Available)
	assert.NotContains(t, synced, true, "no outdated badge right after the update")
}

func TestRunVersionCheck_NoRace_SyncsTheRecordedAnswer(t *testing.T) {
	testCases := []struct {
		name   string
		answer *domain.Available
		want   []bool
	}{
		{name: "something ahead badges outdated", answer: ahead(), want: []bool{true}},
		{name: "nothing ahead clears the badge", want: []bool{false}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			axArrow := newTestAsynxArrow(t)
			ns := stableNs()
			seedSelectorRow(t, axArrow, ns, domain.SelectorChannel, domain.Resolved{Ref: "v1.0.0", Commit: "c100"})
			row, err := axArrow.Get(ctx, ns.String())
			require.NoError(t, err)
			r := &arrowStoreMocks.MockCQRS{
				CheckDriftFn: func(context.Context, domain.Arrow) (*domain.Available, bool) { return tc.answer, true },
			}
			var synced []bool
			cat := arrowRepo.NewTestable(r, axArrow, nil, nil,
				arrowRepo.WithVersionOutdatedSync(func(_ context.Context, _ domain.Namespace, outdated bool) error {
					synced = append(synced, outdated)
					return nil
				}))

			arrowRepo.RunVersionCheckForTest(cat, ctx, row)

			assert.Equal(t, tc.want, synced)
			got, err := axArrow.Get(ctx, ns.String())
			require.NoError(t, err)
			assert.Equal(t, tc.answer, got.Available)
		})
	}
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

// The passive check reads the remote between reading the row and writing its
// answer. An update's advance landing in that window must not leave the
// installed commit recorded as available, nor badge the runtime outdated.
func TestRunVersionCheck_AdvanceLandsWhileJudging_RecordsNothingStale(t *testing.T) {
	ctx := context.Background()
	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	ns := stableNs()
	seedSelectorRow(t, axArrow, ns, domain.SelectorChannel, domain.Resolved{Ref: "v1.0.0", Commit: "c100"})
	require.NoError(t, runtimeRepo.MarkPreinstalled(axRuntime)(ctx, ns))
	stale, err := axArrow.Get(ctx, ns.String())
	require.NoError(t, err)

	target := domain.Available{Ref: "v1.1.0", Commit: "c110"}
	var advanced atomic.Bool
	r := &arrowStoreMocks.MockCQRS{
		CheckDriftFn: func(_ context.Context, judged domain.Arrow) (*domain.Available, bool) {
			if !advanced.Swap(true) {
				_, sendErr := axArrow.SendWait(ctx, arrowcmds.AdvanceArrow{
					Namespace: ns,
					Resolved:  domain.Resolved{Ref: target.Ref, Commit: target.Commit},
				})
				require.NoError(t, sendErr)
			}
			if judged.Resolved.Commit == target.Commit {
				return nil, true
			}
			return &target, true
		},
	}
	cat := arrowRepo.NewTestable(r, axArrow, nil, nil,
		arrowRepo.WithVersionOutdatedSync(runtimeRepo.SetVersionOutdated(axRuntime)))

	arrowRepo.RunVersionCheckForTest(cat, ctx, stale)

	got, err := axArrow.Get(ctx, ns.String())
	require.NoError(t, err)
	assert.Nil(t, got.Available, "the advanced row is current")
	assert.Equal(t, domain.ArrowStateReady, runtimeState(t, axRuntime, ns), "no outdated badge")
}

func TestRunVersionCheck_RowUnreadableAfterWrite_SyncsNothing(t *testing.T) {
	var reads atomic.Int32
	ax := &arrowMocks.AsynxArrow{
		GetFn: func(context.Context, string) (domain.Arrow, error) {
			if reads.Add(1) > 1 {
				return domain.Arrow{}, errors.New("event store down")
			}
			return domain.Arrow{SelectorKind: domain.SelectorChannel}, nil
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

	assert.Empty(t, synced, "a badge is only ever derived from a row actually read")
}
