package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// MockCQRS is a test double for arrowstore.Store.
type MockCQRS struct {
	ListFn              func(ctx context.Context, userInstalled *bool) ([]models.ArrowView, error)
	GetFn               func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)
	GetDetailFn         func(ctx context.Context, ns domain.Namespace) (*models.ArrowDetailView, error)
	GetManifestFn       func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)
	ResolveManifestFn   func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)
	ResolveForInstallFn func(ctx context.Context, ns domain.Namespace) (domain.Namespace, *domain.Arrow, string, error)
	SearchFn            func(ctx context.Context, q models.SearchQuery) ([]models.CatalogHit, error)
	ProjectFn           func(ctx context.Context, arrow domain.Arrow) error
	ProjectForgetFn     func(ctx context.Context, arrow domain.Arrow) error
}

func (m *MockCQRS) List(
	ctx context.Context,
	userInstalled *bool,
) ([]models.ArrowView, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, userInstalled)
	}
	return nil, nil
}

func (m *MockCQRS) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockCQRS) GetDetail(
	ctx context.Context,
	ns domain.Namespace,
) (*models.ArrowDetailView, error) {
	if m.GetDetailFn != nil {
		return m.GetDetailFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockCQRS) GetManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if m.GetManifestFn != nil {
		return m.GetManifestFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockCQRS) ResolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if m.ResolveManifestFn != nil {
		return m.ResolveManifestFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockCQRS) ResolveForInstall(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, *domain.Arrow, string, error) {
	if m.ResolveForInstallFn != nil {
		return m.ResolveForInstallFn(ctx, ns)
	}
	return ns, nil, "", nil
}

func (m *MockCQRS) Search(
	ctx context.Context,
	q models.SearchQuery,
) ([]models.CatalogHit, error) {
	if m.SearchFn != nil {
		return m.SearchFn(ctx, q)
	}
	return nil, nil
}

func (m *MockCQRS) Project(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	if m.ProjectFn != nil {
		return m.ProjectFn(ctx, arrow)
	}
	return nil
}

func (m *MockCQRS) ProjectForget(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	if m.ProjectForgetFn != nil {
		return m.ProjectForgetFn(ctx, arrow)
	}
	return nil
}
