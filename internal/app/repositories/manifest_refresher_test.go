package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	repositories "github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	repoarrow "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type refreshingArrow struct {
	repoarrow.Arrow
	row        *domain.Arrow
	getErr     error
	refreshErr error
	staged     []domain.Available
}

func (a *refreshingArrow) Get(
	_ context.Context,
	_ domain.Namespace,
) (*domain.Arrow, error) {
	return a.row, a.getErr
}

func (a *refreshingArrow) RefreshToTarget(
	_ context.Context,
	_ domain.Namespace,
	target domain.Available,
) (*domain.Arrow, error) {
	a.staged = append(a.staged, target)
	return a.row, a.refreshErr
}

func TestManifestRefresher_StagesTheReleaseTheMethodRuns(t *testing.T) {
	installed := domain.Resolved{Ref: "tip", Commit: "c1"}
	ahead := &domain.Available{Ref: "tip", Commit: "c2"}
	testCases := []struct {
		name   string
		method string
		want   domain.Available
	}{
		{name: "install reads the installed release", method: domain.MethodInstall, want: domain.Available{Ref: "tip", Commit: "c1"}},
		{name: "update reads the update target", method: domain.MethodUpdate, want: *ahead},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cat := &refreshingArrow{row: &domain.Arrow{Resolved: installed, Available: ahead}}

			err := repositories.ManifestRefresher(cat)(context.Background(), "github.com/u/r@tip", tc.method)

			require.NoError(t, err)
			assert.Equal(t, []domain.Available{tc.want}, cat.staged)
		})
	}
}

func TestManifestRefresher_Failures(t *testing.T) {
	errBoom := errors.New("boom")
	testCases := []struct {
		name    string
		cat     *refreshingArrow
		method  string
		wantErr error
	}{
		{name: "row cannot be read", cat: &refreshingArrow{getErr: errBoom}, method: domain.MethodInstall, wantErr: errBoom},
		{name: "refresh fails", cat: &refreshingArrow{row: &domain.Arrow{}, refreshErr: errBoom}, method: domain.MethodInstall, wantErr: errBoom},
		{name: "update with nothing ahead", cat: &refreshingArrow{row: &domain.Arrow{}}, method: domain.MethodUpdate, wantErr: apperrors.ErrStateViolation},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := repositories.ManifestRefresher(tc.cat)(context.Background(), "github.com/u/r@tip", tc.method)

			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}
