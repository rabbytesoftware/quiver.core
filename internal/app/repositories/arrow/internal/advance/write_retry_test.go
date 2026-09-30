package advance_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/advance"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

// A final write that loses a race to another append of the row is sent
// again: the vault cache was already swapped, and leaving the row behind it
// would make the two disagree.
func TestWrites_VersionConflict_AreSentAgain(t *testing.T) {
	installed := domain.Resolved{Ref: "nightly-latest", Commit: "c1", Fingerprint: "c1"}
	target := domain.Available{Ref: "nightly-latest", Commit: "c2"}

	testCases := []struct {
		name      string
		conflicts int32
		write     func(ctx context.Context, cat advance.Advancer, ns domain.Namespace) error
		wantSends int32
		wantErr   error
		want      func(t *testing.T, row domain.Arrow)
	}{
		{
			name:      "advance: a conflict the retry wins",
			conflicts: 1,
			write: func(ctx context.Context, cat advance.Advancer, ns domain.Namespace) error {
				return cat.Advance(ctx, ns, target)
			},
			wantSends: 2,
			want: func(t *testing.T, row domain.Arrow) {
				assert.Equal(t, target.Commit, row.Resolved.Commit)
				assert.Equal(t, "Target", row.Name)
			},
		},
		{
			name:      "advance: conflicts that never stop",
			conflicts: 99,
			write: func(ctx context.Context, cat advance.Advancer, ns domain.Namespace) error {
				return cat.Advance(ctx, ns, target)
			},
			wantSends: 3,
			wantErr:   apperrors.ErrStateViolation,
			want: func(t *testing.T, row domain.Arrow) {
				assert.Equal(t, installed, row.Resolved, "a lost advance leaves the row where it was")
				assert.Equal(t, "Old", row.Name)
			},
		},
		{
			name:      "refresh to target: a conflict the retry wins",
			conflicts: 2,
			write: func(ctx context.Context, cat advance.Advancer, ns domain.Namespace) error {
				_, err := cat.RefreshToTarget(ctx, ns, target)
				return err
			},
			wantSends: 3,
			want: func(t *testing.T, row domain.Arrow) {
				assert.Equal(t, "Target", row.Name)
				assert.Equal(t, installed, row.Resolved)
			},
		},
		{
			name:      "refresh to target: conflicts that never stop",
			conflicts: 99,
			write: func(ctx context.Context, cat advance.Advancer, ns domain.Namespace) error {
				_, err := cat.RefreshToTarget(ctx, ns, target)
				return err
			},
			wantSends: 3,
			wantErr:   apperrors.ErrStateViolation,
			want: func(t *testing.T, row domain.Arrow) {
				assert.Equal(t, "Old", row.Name, "a lost refresh leaves the row's manifest where it was")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ns := rollingNs()
			real := newTestAsynxArrow(t)
			seedSelectorRow(t, real, ns, domain.SelectorChannel, installed)
			ax := &conflictingAsynx{Asynx: real, conflicts: tc.conflicts}
			cat := newTestable(&arrowStoreMocks.MockCQRS{}, ax, &mocks.Vault{}, targetManifold())

			err := tc.write(ctx, cat, ns)

			assert.Equal(t, tc.wantSends, ax.sends.Load())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			row, getErr := real.Get(ctx, ns.String())
			require.NoError(t, getErr)
			tc.want(t, row)
		})
	}
}
