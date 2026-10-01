package advance_test

import (
	"context"
	"sync/atomic"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowMocks "github.com/rabbytesoftware/quiver.core/internal/app/mocks"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

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
			cat := newTestable(&arrowStoreMocks.MockCQRS{}, ax, &mocks.Vault{},
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
