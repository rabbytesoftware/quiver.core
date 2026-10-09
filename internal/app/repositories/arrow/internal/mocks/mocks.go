package mocks

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	arrowstore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// MockCQRS is a test double for arrowstore.Store.
type MockCQRS struct {
	ListFn              func(ctx context.Context, userInstalled *bool) ([]models.ArrowView, error)
	GetFn               func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)
	GetDetailFn         func(ctx context.Context, ns domain.Namespace) (*models.ArrowDetailView, error)
	GetManifestFn       func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)
	ResolveManifestFn   func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)
	RefsFn              func(ctx context.Context, ns domain.Namespace) (domain.RefSnapshot, error)
	ResolveCataloguedFn func(ctx context.Context, ns domain.Namespace) (domain.Namespace, error)
	SearchFn            func(ctx context.Context, q models.SearchQuery) ([]models.CatalogHit, error)
	ProjectFn           func(ctx context.Context, arrow domain.Arrow) error
	ProjectForgetFn     func(ctx context.Context, arrow domain.Arrow) error
	NeedsVersionCheckFn func(ctx context.Context, ns domain.Namespace, lastCheckedAt time.Time) (bool, error)
	ResolveInstallFn    func(ctx context.Context, ns domain.Namespace) (domain.Namespace, *domain.Arrow, error)
	ResolveAdoptionFn   func(ctx context.Context, ns domain.Namespace, resolvedRef string) (arrowstore.Adoption, error)
	CheckDriftFn        func(ctx context.Context, arrow domain.Arrow) (*domain.Available, bool)
	CachedAtCommitFn    func(ctx context.Context, identity domain.Namespace, target domain.Available) (*domain.Arrow, vault.ManifestFile, bool)
	RecordAbsentCalls   []domain.Available
	KnownAbsentFn       func(ctx context.Context, identity domain.Namespace, target domain.Available) bool
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

func (m *MockCQRS) Refs(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	if m.RefsFn != nil {
		return m.RefsFn(ctx, ns)
	}
	return domain.RefSnapshot{}, nil
}

func (m *MockCQRS) ResolveCatalogued(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, error) {
	if m.ResolveCataloguedFn != nil {
		return m.ResolveCataloguedFn(ctx, ns)
	}
	return ns, nil
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

func (m *MockCQRS) NeedsVersionCheck(
	ctx context.Context,
	ns domain.Namespace,
	lastCheckedAt time.Time,
) (bool, error) {
	if m.NeedsVersionCheckFn != nil {
		return m.NeedsVersionCheckFn(ctx, ns, lastCheckedAt)
	}
	return false, nil
}

func (m *MockCQRS) ResolveInstall(
	ctx context.Context,
	ns domain.Namespace,
	_ ...arrowstore.InstallOption,
) (domain.Namespace, *domain.Arrow, error) {
	if m.ResolveInstallFn != nil {
		return m.ResolveInstallFn(ctx, ns)
	}
	return ns, &domain.Arrow{Namespace: ns}, nil
}

func (m *MockCQRS) ResolveAdoption(
	ctx context.Context,
	ns domain.Namespace,
	resolvedRef string,
) (arrowstore.Adoption, error) {
	if m.ResolveAdoptionFn != nil {
		return m.ResolveAdoptionFn(ctx, ns, resolvedRef)
	}
	return arrowstore.Adoption{}, nil
}

func (m *MockCQRS) CheckDrift(
	ctx context.Context,
	arrow domain.Arrow,
) (*domain.Available, bool) {
	if m.CheckDriftFn != nil {
		return m.CheckDriftFn(ctx, arrow)
	}
	return nil, false
}

func (m *MockCQRS) CachedAtCommit(
	ctx context.Context,
	identity domain.Namespace,
	target domain.Available,
) (*domain.Arrow, vault.ManifestFile, bool) {
	if m.CachedAtCommitFn != nil {
		return m.CachedAtCommitFn(ctx, identity, target)
	}
	return nil, vault.ManifestFile{}, false
}

func (m *MockCQRS) KnownAbsent(
	ctx context.Context,
	identity domain.Namespace,
	target domain.Available,
) bool {
	if m.KnownAbsentFn != nil {
		return m.KnownAbsentFn(ctx, identity, target)
	}
	return false
}

func (m *MockCQRS) RecordAbsent(
	_ context.Context,
	_ domain.Namespace,
	target domain.Available,
) {
	m.RecordAbsentCalls = append(m.RecordAbsentCalls, target)
}
