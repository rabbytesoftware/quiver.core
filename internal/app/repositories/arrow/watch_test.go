package arrow_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowRepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	arrowStoreMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func TestWatchVersions_TurnedOff_StartsAndStopsNothing(t *testing.T) {
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{}, newTestAsynxArrow(t), &mocks.Vault{}, &mocks.Manifold{},
		arrowRepo.WithVersionCheckInterval(0))

	assert.NotPanics(t, func() {
		cat.WatchVersions(context.Background())
		cat.StopWatchingVersions()
	})
}

func TestTargetUnmovedAndAddDependency_MissingRow_AreReported(t *testing.T) {
	cat := arrowRepo.NewTestable(&arrowStoreMocks.MockCQRS{
		ResolveInstallFn: func(context.Context, domain.Namespace) (domain.Namespace, *domain.Arrow, error) {
			return "", nil, apperrors.ErrNotFound
		},
	}, newTestAsynxArrow(t), &mocks.Vault{}, &mocks.Manifold{})
	ns := domain.Namespace("github.com/user/repo@stable")

	_, err := cat.TargetUnmoved(context.Background(), ns, domain.Available{Ref: "v1", Commit: "c1"})
	require.ErrorIs(t, err, apperrors.ErrNotFound)

	_, err = cat.AddDependency(context.Background(), ns)
	require.ErrorIs(t, err, apperrors.ErrNotFound)
}
