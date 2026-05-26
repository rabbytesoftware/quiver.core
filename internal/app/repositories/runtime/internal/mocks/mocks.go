package mocks

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/assembler"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// MockArrow is a test double for arrowrepo.Arrow.
type MockArrow struct {
	ListFn              func(ctx context.Context, userInstalled *bool) ([]models.ArrowView, error)
	GetFn               func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)
	ExistsFn            func(ctx context.Context, ns domain.Namespace) (bool, error)
	GetDetailFn         func(ctx context.Context, ns domain.Namespace) (*models.ArrowDetailView, error)
	GetManifestFn       func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)
	ResolveManifestFn   func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)
	ResolveForInstallFn func(ctx context.Context, ns domain.Namespace) (domain.Namespace, *domain.Arrow, string, error)
	AddFn               func(ctx context.Context, ns domain.Namespace) error
	AddDepFn            func(ctx context.Context, ns domain.Namespace, arrow *domain.Arrow, constraint string) error
	RemoveFn            func(ctx context.Context, ns domain.Namespace) error
	SeedFn              func(ctx context.Context, ns domain.Namespace, data []byte) error
	ValidateManifestFn  func(ctx context.Context, data []byte) (*models.ValidationResult, error)
	MarkInstalledFn     func(ctx context.Context, ns domain.Namespace, ref string, at time.Time) error
	ForgetFn            func(ctx context.Context, ns domain.Namespace) error
	UpdateManifestFn    func(ctx context.Context, ns domain.Namespace, arrow *domain.Arrow) error
	ResolveConstraintFn func(ctx context.Context, ns domain.Namespace, constraint string) (string, error)
	UpgradeVersionFn    func(ctx context.Context, oldNs, newNs domain.Namespace, constraint string, runtimeAlreadyExists bool) (*domain.Arrow, error)
	ShutdownFn          func(ctx context.Context) error
	HasDependentsFn     func(ctx context.Context, ns domain.Namespace) (bool, error)
	OnArrowAddedFn      func(fn func(ctx context.Context, ns domain.Namespace, arrow domain.Arrow) error) error
	OnArrowUpdatedFn    func(fn func(ctx context.Context, ns domain.Namespace, arrow *domain.Arrow) error) error
	OnArrowRemovedFn    func(fn func(ctx context.Context, ns domain.Namespace) error) error
	OnArrowUpgradedFn   func(fn func(ctx context.Context, arrow domain.Arrow) error) error
}

func (m *MockArrow) List(
	ctx context.Context,
	userInstalled *bool,
) ([]models.ArrowView, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, userInstalled)
	}
	return nil, nil
}

func (m *MockArrow) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockArrow) Exists(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	if m.ExistsFn != nil {
		return m.ExistsFn(ctx, ns)
	}
	return false, nil
}

func (m *MockArrow) GetDetail(
	ctx context.Context,
	ns domain.Namespace,
) (*models.ArrowDetailView, error) {
	if m.GetDetailFn != nil {
		return m.GetDetailFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockArrow) GetManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if m.GetManifestFn != nil {
		return m.GetManifestFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockArrow) ResolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if m.ResolveManifestFn != nil {
		return m.ResolveManifestFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockArrow) ResolveForInstall(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, *domain.Arrow, string, error) {
	if m.ResolveForInstallFn != nil {
		return m.ResolveForInstallFn(ctx, ns)
	}
	return ns, nil, "", nil
}

func (m *MockArrow) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if m.AddFn != nil {
		return m.AddFn(ctx, ns)
	}
	return nil
}

func (m *MockArrow) AddDep(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
	constraint string,
) error {
	if m.AddDepFn != nil {
		return m.AddDepFn(ctx, ns, arrow, constraint)
	}
	return nil
}

func (m *MockArrow) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if m.RemoveFn != nil {
		return m.RemoveFn(ctx, ns)
	}
	return nil
}

func (m *MockArrow) Seed(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) error {
	if m.SeedFn != nil {
		return m.SeedFn(ctx, ns, data)
	}
	return nil
}

func (m *MockArrow) ValidateManifest(
	ctx context.Context,
	data []byte,
) (*models.ValidationResult, error) {
	if m.ValidateManifestFn != nil {
		return m.ValidateManifestFn(ctx, data)
	}
	return &models.ValidationResult{Valid: true}, nil
}

func (m *MockArrow) MarkInstalled(
	ctx context.Context,
	ns domain.Namespace,
	ref string,
	at time.Time,
) error {
	if m.MarkInstalledFn != nil {
		return m.MarkInstalledFn(ctx, ns, ref, at)
	}
	return nil
}

func (m *MockArrow) Forget(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if m.ForgetFn != nil {
		return m.ForgetFn(ctx, ns)
	}
	return nil
}

func (m *MockArrow) UpdateManifest(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
) error {
	if m.UpdateManifestFn != nil {
		return m.UpdateManifestFn(ctx, ns, arrow)
	}
	return nil
}

func (m *MockArrow) ResolveConstraint(
	ctx context.Context,
	ns domain.Namespace,
	constraint string,
) (string, error) {
	if m.ResolveConstraintFn != nil {
		return m.ResolveConstraintFn(ctx, ns, constraint)
	}
	return "", nil
}

func (m *MockArrow) UpgradeVersion(
	ctx context.Context,
	oldNs domain.Namespace,
	newNs domain.Namespace,
	constraint string,
	runtimeAlreadyExists bool,
) (*domain.Arrow, error) {
	if m.UpgradeVersionFn != nil {
		return m.UpgradeVersionFn(ctx, oldNs, newNs, constraint, runtimeAlreadyExists)
	}
	return nil, nil
}

func (m *MockArrow) Shutdown(ctx context.Context) error {
	if m.ShutdownFn != nil {
		return m.ShutdownFn(ctx)
	}
	return nil
}

func (m *MockArrow) OnArrowAdded(
	fn func(ctx context.Context, ns domain.Namespace, arrow domain.Arrow) error,
) error {
	if m.OnArrowAddedFn != nil {
		return m.OnArrowAddedFn(fn)
	}
	return nil
}

func (m *MockArrow) OnArrowUpdated(
	fn func(ctx context.Context, ns domain.Namespace, arrow *domain.Arrow) error,
) error {
	if m.OnArrowUpdatedFn != nil {
		return m.OnArrowUpdatedFn(fn)
	}
	return nil
}

func (m *MockArrow) OnArrowRemoved(
	fn func(ctx context.Context, ns domain.Namespace) error,
) error {
	if m.OnArrowRemovedFn != nil {
		return m.OnArrowRemovedFn(fn)
	}
	return nil
}

func (m *MockArrow) OnArrowUpgraded(
	fn func(ctx context.Context, arrow domain.Arrow) error,
) error {
	if m.OnArrowUpgradedFn != nil {
		return m.OnArrowUpgradedFn(fn)
	}
	return nil
}

// MockAssembler is a test double for assembler.Assembler.
type MockAssembler struct {
	AssembleFn func(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		userVars map[string]string,
	) (assembler.ResolvedExecution, error)
}

func (m *MockAssembler) Assemble(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
) (assembler.ResolvedExecution, error) {
	if m.AssembleFn != nil {
		return m.AssembleFn(ctx, ns, method, userVars)
	}
	return assembler.ResolvedExecution{}, nil
}
