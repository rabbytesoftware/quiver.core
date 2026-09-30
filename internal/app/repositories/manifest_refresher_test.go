package repositories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repositories "github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	repoarrow "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type refreshingArrow struct {
	repoarrow.Arrow
	refreshed  []domain.Namespace
	updated    []*domain.Arrow
	refreshErr error
	updateErr  error
	result     *domain.Arrow
}

func (a *refreshingArrow) RefreshManifest(
	_ context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	a.refreshed = append(a.refreshed, ns)
	return a.result, a.refreshErr
}

func (a *refreshingArrow) UpdateManifest(
	_ context.Context,
	_ domain.Namespace,
	arrow *domain.Arrow,
) error {
	a.updated = append(a.updated, arrow)
	return a.updateErr
}

func TestManifestRefresher_ResolvesAtTheNamespacesOwnRefAndStoresIt(t *testing.T) {
	ns := domain.Namespace("github.com/u/r@tip")
	arrow := &domain.Arrow{Namespace: ns}
	cat := &refreshingArrow{result: arrow}

	err := repositories.ManifestRefresher(cat)(context.Background(), ns)

	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{ns}, cat.refreshed)
	assert.Equal(t, []*domain.Arrow{arrow}, cat.updated)
}

func TestManifestRefresher_Failures(t *testing.T) {
	errBoom := errors.New("boom")
	testCases := []struct {
		name        string
		cat         *refreshingArrow
		wantUpdates int
	}{
		{name: "refresh fails", cat: &refreshingArrow{refreshErr: errBoom}},
		{name: "update fails", cat: &refreshingArrow{result: &domain.Arrow{}, updateErr: errBoom}, wantUpdates: 1},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := repositories.ManifestRefresher(tc.cat)(context.Background(), "github.com/u/r@tip")

			require.Error(t, err)
			assert.ErrorIs(t, err, errBoom)
			assert.Len(t, tc.cat.updated, tc.wantUpdates)
		})
	}
}
