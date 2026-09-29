package exposerinternal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	exposerinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/exposer/internal"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	self    = domain.Namespace("github.com/user/repo@v1.0.0")
	sibling = domain.Namespace("github.com/user/repo@v2.0.0")
	other   = domain.Namespace("github.com/other/repo@v1.0.0")
)

func catalogOf(
	namespaces ...domain.Namespace,
) exposerinternal.ListArrowsFn {
	return func(context.Context) ([]models.ArrowView, error) {
		views := make([]models.ArrowView, 0, len(namespaces))
		for _, ns := range namespaces {
			views = append(views, models.ArrowView{
				Namespace: ns.BareNamespace(),
				Versions:  []models.VersionView{{Namespace: ns}},
			})
		}
		return views, nil
	}
}

func arrowsWith(
	installed map[domain.Namespace]bool,
	errs map[domain.Namespace]error,
) exposerinternal.GetArrowFn {
	return func(_ context.Context, ns domain.Namespace) (*domain.Arrow, error) {
		if err, ok := errs[ns]; ok {
			return nil, err
		}
		if _, ok := installed[ns]; !ok {
			return nil, nil
		}
		arrow := &domain.Arrow{Namespace: ns}
		if installed[ns] {
			arrow.InstalledAt = time.Unix(1, 0)
		}
		return arrow, nil
	}
}

func TestInstalled(t *testing.T) {
	testCases := []struct {
		name       string
		listArrows exposerinternal.ListArrowsFn
		installed  map[domain.Namespace]bool
		errs       map[domain.Namespace]error
		want       bool
	}{
		{
			name:       "no other ref",
			listArrows: catalogOf(self),
			installed:  map[domain.Namespace]bool{self: true},
		},
		{
			name:       "installed sibling ref",
			listArrows: catalogOf(self, sibling),
			installed:  map[domain.Namespace]bool{sibling: true},
			want:       true,
		},
		{
			name:       "catalogued but uninstalled sibling",
			listArrows: catalogOf(self, sibling),
			installed:  map[domain.Namespace]bool{sibling: false},
		},
		{
			name:       "sibling without an arrow",
			listArrows: catalogOf(self, sibling),
		},
		{
			name:       "installed arrow of another namespace",
			listArrows: catalogOf(self, other),
			installed:  map[domain.Namespace]bool{other: true},
		},
		{
			name:       "forgotten sibling",
			listArrows: catalogOf(self, sibling),
			errs:       map[domain.Namespace]error{sibling: asynxModels.ErrNotFound},
		},
		{
			name:       "unreadable sibling counts as installed",
			listArrows: catalogOf(self, sibling),
			errs:       map[domain.Namespace]error{sibling: errors.New("store down")},
			want:       true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := exposerinternal.Installed(context.Background(), self, arrowsWith(tc.installed, tc.errs), tc.listArrows)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestInstalled_CatalogErrorIsReturned(t *testing.T) {
	boom := errors.New("catalog down")

	got, err := exposerinternal.Installed(
		context.Background(),
		self,
		arrowsWith(nil, nil),
		func(context.Context) ([]models.ArrowView, error) { return nil, boom },
	)

	assert.ErrorIs(t, err, boom)
	assert.False(t, got)
}
