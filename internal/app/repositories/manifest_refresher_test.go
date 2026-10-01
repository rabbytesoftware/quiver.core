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

func rememberedTarget(
	target *domain.Available,
) func(domain.Namespace) (domain.Available, bool) {
	return func(domain.Namespace) (domain.Available, bool) {
		if target == nil {
			return domain.Available{}, false
		}
		return *target, true
	}
}

func TestManifestRefresher_StagesTheReleaseTheRunBuilds(t *testing.T) {
	installed := domain.Resolved{Ref: "v1.0.0", Commit: "c1"}
	began := &domain.Available{Ref: "v1.1.0", Commit: "c2"}
	testCases := []struct {
		name      string
		method    string
		available *domain.Available
		want      domain.Available
	}{
		{
			name:   "install reads the installed release",
			method: domain.MethodInstall, available: began,
			want: domain.Available{Ref: "v1.0.0", Commit: "c1"},
		},
		{
			name:   "update reads the target it began toward",
			method: domain.MethodUpdate, available: began,
			want: *began,
		},
		{
			name:   "a newer release recorded during the update is not the one the run builds",
			method: domain.MethodUpdate, available: &domain.Available{Ref: "v1.2.0", Commit: "c3"},
			want: *began,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cat := &refreshingArrow{row: &domain.Arrow{Resolved: installed, Available: tc.available}}

			err := repositories.ManifestRefresher(cat, rememberedTarget(began))(context.Background(), "github.com/u/r@stable", tc.method)

			require.NoError(t, err)
			assert.Equal(t, []domain.Available{tc.want}, cat.staged)
		})
	}
}

func TestManifestRefresher_Failures(t *testing.T) {
	errBoom := errors.New("boom")
	target := &domain.Available{Ref: "v1.1.0", Commit: "c2"}
	testCases := []struct {
		name    string
		cat     *refreshingArrow
		method  string
		target  *domain.Available
		wantErr error
	}{
		{name: "row cannot be read", cat: &refreshingArrow{getErr: errBoom}, method: domain.MethodInstall, wantErr: errBoom},
		{name: "refresh fails", cat: &refreshingArrow{row: &domain.Arrow{}, refreshErr: errBoom}, method: domain.MethodInstall, wantErr: errBoom},
		{name: "update began toward no remembered target", cat: &refreshingArrow{row: &domain.Arrow{Available: target}}, method: domain.MethodUpdate, wantErr: apperrors.ErrStateViolation},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := repositories.ManifestRefresher(tc.cat, rememberedTarget(tc.target))(context.Background(), "github.com/u/r@stable", tc.method)

			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestUpdateTargets_AnswerOnlyOnceTheLifecycleIsSet(t *testing.T) {
	set, lookup := repositories.UpdateTargets()
	target := domain.Available{Ref: "v1.1.0", Commit: "c2"}

	_, ok := lookup("github.com/u/r@stable")
	assert.False(t, ok)

	set(rememberedTarget(&target))
	got, ok := lookup("github.com/u/r@stable")
	assert.True(t, ok)
	assert.Equal(t, target, got)
}
