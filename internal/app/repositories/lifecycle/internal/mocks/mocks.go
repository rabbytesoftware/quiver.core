package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type MockArrow struct {
	ResolveCataloguedFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.Namespace, error)
	GetFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	ExistsFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	CheckAvailableFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Available, error)
	TargetUnmovedFn func(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) (bool, error)
	RefreshToTargetFn func(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) (*domain.Arrow, error)
	AddDependencyFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.Namespace, error)
	AdvanceFn func(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) error
}

// ResolveCatalogued defaults to the identity so tests that predate namespace
// resolution keep exercising the namespace they passed in.
func (m *MockArrow) ResolveCatalogued(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, error) {
	if m.ResolveCataloguedFn != nil {
		return m.ResolveCataloguedFn(ctx, ns)
	}
	return ns, nil
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

func (m *MockArrow) CheckAvailable(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Available, error) {
	if m.CheckAvailableFn != nil {
		return m.CheckAvailableFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockArrow) TargetUnmoved(
	ctx context.Context,
	ns domain.Namespace,
	target domain.Available,
) (bool, error) {
	if m.TargetUnmovedFn != nil {
		return m.TargetUnmovedFn(ctx, ns, target)
	}
	return true, nil
}

func (m *MockArrow) RefreshToTarget(
	ctx context.Context,
	ns domain.Namespace,
	target domain.Available,
) (*domain.Arrow, error) {
	if m.RefreshToTargetFn != nil {
		return m.RefreshToTargetFn(ctx, ns, target)
	}
	return &domain.Arrow{Namespace: ns}, nil
}

func (m *MockArrow) AddDependency(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, error) {
	if m.AddDependencyFn != nil {
		return m.AddDependencyFn(ctx, ns)
	}
	return ns, nil
}

func (m *MockArrow) Advance(
	ctx context.Context,
	ns domain.Namespace,
	target domain.Available,
) error {
	if m.AdvanceFn != nil {
		return m.AdvanceFn(ctx, ns, target)
	}
	return nil
}

type MockRuntime struct {
	BeginInstallFn func(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) error
	BeginExecutionFn func(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		vars map[string]string,
	) error
	BeginStopFn func(
		ctx context.Context,
		ns domain.Namespace,
	) error
	BeginUninstallFn func(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) error
	BeginUpdateFn func(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
		targetRef string,
	) error
	OnRuntimeEndedFn func(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	GetStateFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.ArrowState, error)
	GetRuntimeFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domainRuntime.ArrowRuntime, error)
	ListenEndedFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (<-chan domainRuntime.ArrowRuntime, func(), error)
	MarkOutdatedFn func(
		ctx context.Context,
		ns domain.Namespace,
		addedDeps []domain.Namespace,
		removedDeps []domain.Namespace,
	) error
	ReconcileVersionBadgeFn func(
		ctx context.Context,
		ns domain.Namespace,
	) error
	ForgetFn func(
		ctx context.Context,
		ns domain.Namespace,
	) error
	ForgottenNamespaces []domain.Namespace
	ForgetErr           error
}

func (m *MockRuntime) BeginInstall(ctx context.Context, ns domain.Namespace, vars map[string]string) error {
	if m.BeginInstallFn != nil {
		return m.BeginInstallFn(ctx, ns, vars)
	}
	return nil
}

func (m *MockRuntime) BeginExecution(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	vars map[string]string,
) error {
	if m.BeginExecutionFn != nil {
		return m.BeginExecutionFn(ctx, ns, method, vars)
	}
	return nil
}

func (m *MockRuntime) BeginStop(ctx context.Context, ns domain.Namespace) error {
	if m.BeginStopFn != nil {
		return m.BeginStopFn(ctx, ns)
	}
	return nil
}

func (m *MockRuntime) BeginUninstall(ctx context.Context, ns domain.Namespace, vars map[string]string) error {
	if m.BeginUninstallFn != nil {
		return m.BeginUninstallFn(ctx, ns, vars)
	}
	return nil
}

func (m *MockRuntime) BeginUpdate(ctx context.Context, ns domain.Namespace, vars map[string]string, targetRef string) error {
	if m.BeginUpdateFn != nil {
		return m.BeginUpdateFn(ctx, ns, vars, targetRef)
	}
	return nil
}

func (m *MockRuntime) OnRuntimeEnded(
	fn func(ctx context.Context, rt domainRuntime.ArrowRuntime),
) error {
	if m.OnRuntimeEndedFn != nil {
		return m.OnRuntimeEndedFn(fn)
	}
	return nil
}

func (m *MockRuntime) GetState(
	ctx context.Context,
	ns domain.Namespace,
) (domain.ArrowState, error) {
	if m.GetStateFn != nil {
		return m.GetStateFn(ctx, ns)
	}
	return domain.ArrowStateAbsent, nil
}

func (m *MockRuntime) GetRuntime(
	ctx context.Context,
	ns domain.Namespace,
) (*domainRuntime.ArrowRuntime, error) {
	if m.GetRuntimeFn != nil {
		return m.GetRuntimeFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockRuntime) ListenEnded(
	ctx context.Context,
	ns domain.Namespace,
) (<-chan domainRuntime.ArrowRuntime, func(), error) {
	if m.ListenEndedFn != nil {
		return m.ListenEndedFn(ctx, ns)
	}
	ch := make(chan domainRuntime.ArrowRuntime, 1)
	return ch, func() {}, nil
}

func (m *MockRuntime) MarkOutdated(
	ctx context.Context,
	ns domain.Namespace,
	addedDeps []domain.Namespace,
	removedDeps []domain.Namespace,
) error {
	if m.MarkOutdatedFn != nil {
		return m.MarkOutdatedFn(ctx, ns, addedDeps, removedDeps)
	}
	return nil
}

func (m *MockRuntime) ReconcileVersionBadge(ctx context.Context, ns domain.Namespace) error {
	if m.ReconcileVersionBadgeFn != nil {
		return m.ReconcileVersionBadgeFn(ctx, ns)
	}
	return nil
}

func (m *MockRuntime) Forget(ctx context.Context, ns domain.Namespace) error {
	m.ForgottenNamespaces = append(m.ForgottenNamespaces, ns)
	if m.ForgetFn != nil {
		return m.ForgetFn(ctx, ns)
	}
	if m.ForgetErr != nil {
		return m.ForgetErr
	}
	return nil
}

type MockGraph struct {
	ResolveFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (models.Plan, error)
	HasDependentsFn func(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (bool, error)
	GetDependentsFn func(
		ctx context.Context,
		ns domain.Namespace,
	) ([]domain.Namespace, error)
	DiffDepsFn func(old, new *domain.Arrow) models.DepDiff
}

func (m *MockGraph) Resolve(
	ctx context.Context,
	ns domain.Namespace,
) (models.Plan, error) {
	if m.ResolveFn != nil {
		return m.ResolveFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockGraph) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	if m.HasDependentsFn != nil {
		return m.HasDependentsFn(ctx, ns, excludeNs)
	}
	return false, nil
}

func (m *MockGraph) GetDependents(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Namespace, error) {
	if m.GetDependentsFn != nil {
		return m.GetDependentsFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockGraph) DiffDeps(
	old, new *domain.Arrow,
) models.DepDiff {
	if m.DiffDepsFn != nil {
		return m.DiffDepsFn(old, new)
	}
	return models.DepDiff{}
}
