package mocks

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/graph"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type MockArrow struct {
	ListFn func(
		ctx context.Context,
		userInstalled *bool,
	) ([]models.ArrowView, error)
	GetFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	ExistsFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	GetDetailFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (*models.ArrowDetailView, error)
	GetManifestFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	ResolveManifestFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	ResolveForInstallFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.Namespace, *domain.Arrow, string, error)
	AddFn func(
		ctx context.Context,
		ns domain.Namespace,
	) error
	AddDepFn func(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
		constraint string,
	) error
	RemoveFn func(
		ctx context.Context,
		ns domain.Namespace,
	) error
	SeedFn func(
		ctx context.Context,
		ns domain.Namespace,
		data []byte,
	) error
	ValidateManifestFn func(
		ctx context.Context,
		data []byte,
	) (*models.ValidationResult, error)
	MarkInstalledFn func(
		ctx context.Context,
		ns domain.Namespace,
		ref string,
		at time.Time,
	) error
	ForgetFn func(
		ctx context.Context,
		ns domain.Namespace,
	) error
	UpdateManifestFn func(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
	) error
	ResolveConstraintFn func(
		ctx context.Context,
		ns domain.Namespace,
		constraint string,
	) (string, error)
	UpgradeVersionFn func(
		ctx context.Context,
		oldNs domain.Namespace,
		newNs domain.Namespace,
		constraint string,
		runtimeAlreadyExists bool,
	) (*domain.Arrow, error)
	ShutdownFn     func(ctx context.Context) error
	OnArrowAddedFn func(fn func(
		ctx context.Context,
		ns domain.Namespace,
		arrow domain.Arrow,
	) error) error
	OnArrowUpdatedFn func(fn func(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
	) error) error
	OnArrowRemovedFn func(fn func(
		ctx context.Context,
		ns domain.Namespace,
	) error) error
	OnArrowUpgradedFn func(fn func(
		ctx context.Context,
		arrow domain.Arrow,
	) error) error
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
	return "", nil, "", nil
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
	return nil, nil
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

type MockRuntime struct {
	BeginExecutionFn func(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		vars map[string]string,
	) error
	UninstallFn func(
		ctx context.Context,
		ns domain.Namespace,
		userVars map[string]string,
	) error
	StopFn func(
		ctx context.Context,
		ns domain.Namespace,
	) error
	RuntimeExistsFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	StartFn          func(ctx context.Context)
	ShutdownFn       func(ctx context.Context) error
	OnRuntimeEndedFn func(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimeBegunFn func(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimeRecoveredFn func(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimeDetachedFn func(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimePIDRecordedFn func(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimeOutdatedFn func(fn func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)) error
	OnRuntimeOutdatedClearedFn func(fn func(
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
	ClearOutdatedFn func(
		ctx context.Context,
		ns domain.Namespace,
	) error
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

func (m *MockRuntime) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	if m.UninstallFn != nil {
		return m.UninstallFn(ctx, ns, userVars)
	}
	return nil
}

func (m *MockRuntime) Stop(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if m.StopFn != nil {
		return m.StopFn(ctx, ns)
	}
	return nil
}

func (m *MockRuntime) RuntimeExists(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	if m.RuntimeExistsFn != nil {
		return m.RuntimeExistsFn(ctx, ns)
	}
	return false, nil
}

func (m *MockRuntime) Start(ctx context.Context) {
	if m.StartFn != nil {
		m.StartFn(ctx)
	}
}

func (m *MockRuntime) Shutdown(ctx context.Context) error {
	if m.ShutdownFn != nil {
		return m.ShutdownFn(ctx)
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

func (m *MockRuntime) OnRuntimeBegun(
	fn func(ctx context.Context, rt domainRuntime.ArrowRuntime),
) error {
	if m.OnRuntimeBegunFn != nil {
		return m.OnRuntimeBegunFn(fn)
	}
	return nil
}

func (m *MockRuntime) OnRuntimeRecovered(
	fn func(ctx context.Context, rt domainRuntime.ArrowRuntime),
) error {
	if m.OnRuntimeRecoveredFn != nil {
		return m.OnRuntimeRecoveredFn(fn)
	}
	return nil
}

func (m *MockRuntime) OnRuntimeDetached(
	fn func(ctx context.Context, rt domainRuntime.ArrowRuntime),
) error {
	if m.OnRuntimeDetachedFn != nil {
		return m.OnRuntimeDetachedFn(fn)
	}
	return nil
}

func (m *MockRuntime) OnRuntimePIDRecorded(
	fn func(ctx context.Context, rt domainRuntime.ArrowRuntime),
) error {
	if m.OnRuntimePIDRecordedFn != nil {
		return m.OnRuntimePIDRecordedFn(fn)
	}
	return nil
}

func (m *MockRuntime) OnRuntimeOutdated(
	fn func(ctx context.Context, rt domainRuntime.ArrowRuntime),
) error {
	if m.OnRuntimeOutdatedFn != nil {
		return m.OnRuntimeOutdatedFn(fn)
	}
	return nil
}

func (m *MockRuntime) OnRuntimeOutdatedCleared(
	fn func(ctx context.Context, rt domainRuntime.ArrowRuntime),
) error {
	if m.OnRuntimeOutdatedClearedFn != nil {
		return m.OnRuntimeOutdatedClearedFn(fn)
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

func (m *MockRuntime) ClearOutdated(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if m.ClearOutdatedFn != nil {
		return m.ClearOutdatedFn(ctx, ns)
	}
	return nil
}

type MockGraph struct {
	ResolveFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (graph.Plan, error)
	UnplanFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (graph.Plan, error)
	HasDependentsFn func(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (bool, error)
	OrphansFn func(
		ctx context.Context,
		ns domain.Namespace,
	) ([]domain.Namespace, error)
	GetDependentsFn func(
		ctx context.Context,
		ns domain.Namespace,
	) ([]domain.Namespace, error)
	SyncDependenciesFn func(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
	) error
	RemoveDependenciesFn func(
		ctx context.Context,
		ns domain.Namespace,
	) error
	DiffDepsFn func(old, new *domain.Arrow) graph.DepDiff
}

func (m *MockGraph) Resolve(
	ctx context.Context,
	ns domain.Namespace,
) (graph.Plan, error) {
	if m.ResolveFn != nil {
		return m.ResolveFn(ctx, ns)
	}
	return nil, nil
}

func (m *MockGraph) Unplan(
	ctx context.Context,
	ns domain.Namespace,
) (graph.Plan, error) {
	if m.UnplanFn != nil {
		return m.UnplanFn(ctx, ns)
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

func (m *MockGraph) Orphans(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Namespace, error) {
	if m.OrphansFn != nil {
		return m.OrphansFn(ctx, ns)
	}
	return nil, nil
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

func (m *MockGraph) SyncDependencies(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
) error {
	if m.SyncDependenciesFn != nil {
		return m.SyncDependenciesFn(ctx, ns, arrow)
	}
	return nil
}

func (m *MockGraph) RemoveDependencies(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if m.RemoveDependenciesFn != nil {
		return m.RemoveDependenciesFn(ctx, ns)
	}
	return nil
}

func (m *MockGraph) DiffDeps(
	old, new *domain.Arrow,
) graph.DepDiff {
	if m.DiffDepsFn != nil {
		return m.DiffDepsFn(old, new)
	}
	return graph.DepDiff{}
}

type MockQuiver struct {
	FollowFn func(
		ctx context.Context,
		ns domain.Namespace,
	) error
	UnfollowFn func(
		ctx context.Context,
		ns domain.Namespace,
	) error
	ListFn func(ctx context.Context) ([]domain.Quiver, error)
	GetFn  func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.QuiverManifest, map[domain.Namespace][]byte, error)
	UpdateFailedArrowsFn func(
		ctx context.Context,
		ns domain.Namespace,
		failedArrows []domain.Namespace,
	) error
	IsFollowedFn func(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	OnQuiverFollowedFn func(fn func(
		ctx context.Context,
		q domain.Quiver,
	)) error
	OnQuiverUnfollowedFn func(fn func(
		ctx context.Context,
		ns domain.Namespace,
	)) error
}

func (m *MockQuiver) Follow(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if m.FollowFn != nil {
		return m.FollowFn(ctx, ns)
	}
	return nil
}

func (m *MockQuiver) Unfollow(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if m.UnfollowFn != nil {
		return m.UnfollowFn(ctx, ns)
	}
	return nil
}

func (m *MockQuiver) List(ctx context.Context) ([]domain.Quiver, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx)
	}
	return nil, nil
}

func (m *MockQuiver) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.QuiverManifest, map[domain.Namespace][]byte, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, ns)
	}
	return &domain.QuiverManifest{}, map[domain.Namespace][]byte{}, nil
}

func (m *MockQuiver) UpdateFailedArrows(
	ctx context.Context,
	ns domain.Namespace,
	failedArrows []domain.Namespace,
) error {
	if m.UpdateFailedArrowsFn != nil {
		return m.UpdateFailedArrowsFn(ctx, ns, failedArrows)
	}
	return nil
}

func (m *MockQuiver) IsFollowed(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	if m.IsFollowedFn != nil {
		return m.IsFollowedFn(ctx, ns)
	}
	return false, nil
}

func (m *MockQuiver) OnQuiverFollowed(fn func(
	ctx context.Context,
	q domain.Quiver,
),
) error {
	if m.OnQuiverFollowedFn != nil {
		return m.OnQuiverFollowedFn(fn)
	}
	return nil
}

func (m *MockQuiver) OnQuiverUnfollowed(fn func(
	ctx context.Context,
	ns domain.Namespace,
),
) error {
	if m.OnQuiverUnfollowedFn != nil {
		return m.OnQuiverUnfollowedFn(fn)
	}
	return nil
}
