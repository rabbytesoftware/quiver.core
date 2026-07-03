package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowrepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	runtimerepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type RuntimeUsecase interface {
	Install(
		ctx context.Context,
		ns domain.Namespace,
		userVars map[string]string,
	) error
	Uninstall(
		ctx context.Context,
		ns domain.Namespace,
		userVars map[string]string,
	) error
	Execute(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		userVars map[string]string,
	) error
	Stop(
		ctx context.Context,
		ns domain.Namespace,
	) error
	RuntimeExists(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	GetRuntime(
		ctx context.Context,
		ns domain.Namespace,
	) (*domainRuntime.ArrowRuntime, error)
	ListRuntimes(
		ctx context.Context,
	) ([]domainRuntime.ArrowRuntime, error)
	Start(ctx context.Context)
}

type runtimeUsecase struct {
	arrow   arrowrepo.Arrow
	runtime runtimerepo.Runtime
	graph   graph.Graph
}

func NewRuntimeUsecase(
	arrow arrowrepo.Arrow,
	runtime runtimerepo.Runtime,
	graph graph.Graph,
) RuntimeUsecase {
	return &runtimeUsecase{
		arrow:   arrow,
		runtime: runtime,
		graph:   graph,
	}
}

// rejectReservedVariables refuses a request that tries to set a name Quiver
// computes. Dropping the value silently would hand the caller the very
// asked-for-X-got-Y failure the built-ins exist to prevent.
func rejectReservedVariables(
	userVars map[string]string,
) error {
	for _, name := range domain.ReservedVariableNames() {
		if _, ok := userVars[name]; ok {
			return fmt.Errorf("%w: %q", apperrors.ErrReservedVariable, name)
		}
	}
	return nil
}

func (u *runtimeUsecase) Install( //nolint:gocyclo
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	if err := rejectReservedVariables(userVars); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	exists, err := u.arrow.Exists(ctx, ns)
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	if !exists {
		return fmt.Errorf("install: %w", apperrors.ErrNotFound)
	}

	plan, err := u.graph.Resolve(ctx, ns)
	if err != nil {
		return fmt.Errorf("install: resolve deps: %w", err)
	}

	for _, entry := range plan {
		depExists, depErr := u.arrow.Exists(ctx, entry.Namespace)
		if depErr != nil {
			return fmt.Errorf("install: check dep %s: %w", entry.Namespace, depErr)
		}
		if !depExists { //nolint:nestif
			resolvedNs, arrow, constraint, resolveErr := u.arrow.ResolveForInstall(ctx, entry.Namespace)
			if resolveErr != nil {
				return fmt.Errorf("install: resolve dep manifest %s: %w", entry.Namespace, resolveErr)
			}
			arrow.UserInstalled = false
			if addErr := u.arrow.AddDep(ctx, resolvedNs, arrow, constraint); addErr != nil &&
				!errors.Is(addErr, apperrors.ErrAlreadyExists) {
				return fmt.Errorf("install: add dep to catalog %s: %w", entry.Namespace, addErr)
			}
		}
	}

	for _, entry := range plan {
		if err := u.installOneDep(ctx, entry.Namespace); err != nil {
			return err
		}
		if entry.Type == domain.ServiceDep {
			if err := u.startServiceDep(ctx, entry.Namespace); err != nil {
				return err
			}
		}
	}

	state, err := u.runtime.GetState(ctx, ns)
	if err != nil {
		return fmt.Errorf("install: get state: %w", err)
	}
	if state != "" &&
		state != domain.ArrowStateAbsent &&
		state != domain.ArrowStateInstalling &&
		state != domain.ArrowStateRemoved {
		return nil
	}

	return u.runtime.BeginInstall(ctx, ns, userVars)
}

func (u *runtimeUsecase) installOneDep(ctx context.Context, depNs domain.Namespace) error {
	ch, unsub, err := u.runtime.ListenEnded(ctx, depNs)
	if err != nil {
		return fmt.Errorf("install dep %s: listen: %w", depNs, err)
	}
	defer unsub()

	state, stateErr := u.runtime.GetState(ctx, depNs)
	if stateErr != nil {
		return fmt.Errorf("install dep %s: get state: %w", depNs, stateErr)
	}

	if state != "" &&
		state != domain.ArrowStateAbsent &&
		state != domain.ArrowStateInstalling &&
		state != domain.ArrowStateRemoved {
		return nil
	}

	if beErr := u.runtime.BeginInstall(ctx, depNs, nil); beErr != nil {
		if !errors.Is(beErr, apperrors.ErrStateViolation) {
			return beErr
		}
	}

	select {
	case rt := <-ch:
		if rt.LastReturn != nil && rt.LastReturn.Outcome != domainRuntime.ExecutionOutcomeSuccess {
			return fmt.Errorf("install dep %s: install failed", depNs)
		}
		return nil
	case <-ctx.Done():
		slog.ErrorContext(ctx, "install dep: timeout waiting for install", "dep", depNs)
		return ctx.Err()
	}
}

func (u *runtimeUsecase) startServiceDep(ctx context.Context, depNs domain.Namespace) error {
	state, err := u.runtime.GetState(ctx, depNs)
	if err != nil {
		return fmt.Errorf("start service dep %s: get state: %w", depNs, err)
	}
	if state == domain.ArrowStateRunning {
		return nil
	}

	if beErr := u.runtime.BeginExecution(ctx, depNs, domain.MethodExecute, nil); beErr != nil {
		if !errors.Is(beErr, apperrors.ErrStateViolation) {
			return fmt.Errorf("start service dep %s: %w", depNs, beErr)
		}
	}
	return nil
}

func (u *runtimeUsecase) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	if err := rejectReservedVariables(userVars); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}

	hasDeps, err := u.graph.HasDependents(ctx, ns, "")
	if err != nil {
		return err
	}
	if hasDeps {
		return fmt.Errorf("uninstall: %w", apperrors.ErrDependentsExist)
	}
	return u.runtime.BeginUninstall(ctx, ns, userVars)
}

func (u *runtimeUsecase) Execute(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	userVars map[string]string,
) error {
	if err := rejectReservedVariables(userVars); err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	if method == domain.MethodUpdate {
		return u.executeUpdate(ctx, ns, userVars)
	}
	return u.runtime.BeginExecution(ctx, ns, method, userVars)
}

func (u *runtimeUsecase) executeUpdate(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	state, err := u.runtime.GetState(ctx, ns)
	if err != nil {
		return fmt.Errorf("execute: get state: %w", err)
	}
	if state == domain.ArrowStateOutdated {
		if err := u.syncDeps(ctx, ns); err != nil {
			return fmt.Errorf("execute: sync deps: %w", err)
		}
		return u.runtime.BeginUpdate(ctx, ns, userVars)
	}
	if state == domain.ArrowStateReady {
		return u.runtime.BeginUpdate(ctx, ns, userVars)
	}
	return u.runtime.BeginExecution(ctx, ns, domain.MethodUpdate, userVars)
}

func (u *runtimeUsecase) Stop(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return u.runtime.BeginStop(ctx, ns)
}

func (u *runtimeUsecase) syncDeps( //nolint:gocyclo
	ctx context.Context,
	ns domain.Namespace,
) error {
	rt, err := u.runtime.GetRuntime(ctx, ns)
	if err != nil {
		return fmt.Errorf("sync deps: %w", err)
	}
	if rt == nil || rt.State != domain.ArrowStateOutdated {
		return fmt.Errorf("sync deps: %w", apperrors.ErrStateViolation)
	}

	syncInfo := rt.PendingDepSync
	if syncInfo == nil {
		return nil
	}

	for _, depNs := range syncInfo.AddedDeps {
		depExists, depErr := u.arrow.Exists(ctx, depNs)
		if depErr != nil {
			return fmt.Errorf("sync deps: check dep %s: %w", depNs, depErr)
		}
		if !depExists { //nolint:nestif
			resolvedNs, arrow, constraint, resolveErr := u.arrow.ResolveForInstall(ctx, depNs)
			if resolveErr != nil {
				return fmt.Errorf("sync deps: resolve dep %s: %w", depNs, resolveErr)
			}
			arrow.UserInstalled = false
			if addErr := u.arrow.AddDep(ctx, resolvedNs, arrow, constraint); addErr != nil &&
				!errors.Is(addErr, apperrors.ErrAlreadyExists) {
				return fmt.Errorf("sync deps: add dep %s: %w", depNs, addErr)
			}
		}
	}

	for _, depNs := range syncInfo.AddedDeps {
		if err := u.installOneDep(ctx, depNs); err != nil {
			return err
		}
	}

	plan, err := u.graph.Resolve(ctx, ns)
	if err == nil { //nolint:nestif
		planMap := make(map[domain.Namespace]domain.DepType, len(plan))
		for _, entry := range plan {
			planMap[entry.Namespace] = entry.Type
		}
		for _, depNs := range syncInfo.AddedDeps {
			if planMap[depNs] == domain.ServiceDep {
				if startErr := u.startServiceDep(ctx, depNs); startErr != nil {
					return fmt.Errorf("sync deps: start service dep %s: %w", depNs, startErr)
				}
			}
		}
	}

	for _, depNs := range syncInfo.RemovedDeps {
		arrow, getErr := u.arrow.Get(ctx, depNs)
		if getErr != nil || arrow == nil || arrow.UserInstalled {
			continue
		}
		hasDeps, depsErr := u.graph.HasDependents(ctx, depNs, "")
		if depsErr != nil || hasDeps {
			continue
		}
		depState, stateErr := u.runtime.GetState(ctx, depNs)
		if stateErr != nil {
			continue
		}
		switch depState {
		case domain.ArrowStateReady, domain.ArrowStateOutdated:
			_ = u.runtime.BeginUninstall(ctx, depNs, nil)
		case domain.ArrowStateRunning, domain.ArrowStateStopping:
			_ = u.runtime.BeginStop(ctx, depNs)
		case domain.ArrowStateAbsent,
			domain.ArrowStateInstalling,
			domain.ArrowStateUpdating,
			domain.ArrowStateDraining,
			domain.ArrowStateDetached,
			domain.ArrowStateUninstalling,
			domain.ArrowStateRemoved:
		}
	}

	return nil
}

func (u *runtimeUsecase) RuntimeExists(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	return u.runtime.RuntimeExists(ctx, ns)
}

func (u *runtimeUsecase) GetRuntime(
	ctx context.Context,
	ns domain.Namespace,
) (*domainRuntime.ArrowRuntime, error) {
	rt, err := u.runtime.GetRuntime(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get runtime %s: %w", ns, err)
	}
	if rt != nil {
		return rt, nil
	}

	exists, err := u.arrow.Exists(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get runtime: check arrow %s: %w", ns, err)
	}
	if !exists {
		return nil, fmt.Errorf("get runtime %s: %w", ns, apperrors.ErrNotFound)
	}

	return &domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateAbsent}, nil
}

func (u *runtimeUsecase) ListRuntimes(
	ctx context.Context,
) ([]domainRuntime.ArrowRuntime, error) {
	views, err := u.arrow.List(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list runtimes: list arrows: %w", err)
	}

	runtimes := make([]domainRuntime.ArrowRuntime, 0, len(views))
	for _, v := range views {
		rt, err := u.runtime.GetRuntime(ctx, v.Namespace)
		if err != nil {
			return nil, fmt.Errorf("list runtimes: get runtime %s: %w", v.Namespace, err)
		}
		if rt == nil {
			rt = &domainRuntime.ArrowRuntime{Ref: v.Namespace, State: domain.ArrowStateAbsent}
		}
		runtimes = append(runtimes, *rt)
	}

	return runtimes, nil
}

func (u *runtimeUsecase) Start(ctx context.Context) {
	u.runtime.Start(ctx)
}

func (u *runtimeUsecase) onArrowUpgraded(ctx context.Context, arrow domain.Arrow) {
	oldNs := arrow.UpgradedFromNs
	newNs := arrow.Namespace

	current, err := u.arrow.Get(ctx, oldNs)
	if err != nil || current == nil {
		return
	}

	diff := u.graph.DiffDeps(current, &arrow)
	oldState, _ := u.runtime.GetState(ctx, oldNs)

	_ = u.arrow.Remove(ctx, oldNs)

	if oldState != domain.ArrowStateReady {
		return
	}

	if len(diff.Added) > 0 || len(diff.Removed) > 0 {
		_ = u.runtime.MarkOutdated(ctx, newNs, edgesToNs(diff.Added), edgesToNs(diff.Removed))
	} else {
		_ = u.runtime.BeginInstall(ctx, newNs, nil)
	}
}

func (u *runtimeUsecase) onRuntimeEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	if rt.LastReturn == nil {
		return
	}

	switch rt.LastReturn.Method {
	case domain.MethodStop:
		u.onStopEnded(ctx, rt)
	case domain.MethodUninstall:
		u.onUninstallEnded(ctx, rt)
	}
}

func (u *runtimeUsecase) onStopEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	plan, err := u.graph.Resolve(ctx, rt.Ref)
	if err != nil {
		return
	}

	for _, entry := range plan {
		if entry.Type != domain.ServiceDep {
			continue
		}
		depNs := entry.Namespace
		state, stateErr := u.runtime.GetState(ctx, depNs)
		if stateErr != nil {
			continue
		}
		if state != domain.ArrowStateRunning && state != domain.ArrowStateStopping {
			continue
		}

		parents, parentsErr := u.graph.GetDependents(ctx, depNs)
		if parentsErr != nil {
			slog.ErrorContext(ctx, "onStopEnded: GetDependents failed", "ns", depNs, "err", parentsErr)
			continue
		}

		filteredParents := make([]domain.Namespace, 0, len(parents))
		for _, p := range parents {
			if p != rt.Ref {
				filteredParents = append(filteredParents, p)
			}
		}

		if countRunning(ctx, filteredParents, u.runtime.GetState) == 0 {
			_ = u.runtime.BeginStop(ctx, depNs)
		}
	}

	// After cascading stops, check if the arrow that just stopped is itself
	// an orphaned non-user-installed dep that should be auto-uninstalled.
	u.maybeAutoUninstallStopped(ctx, rt.Ref)
}

func (u *runtimeUsecase) maybeAutoUninstallStopped(ctx context.Context, ns domain.Namespace) {
	arrow, err := u.arrow.Get(ctx, ns)
	if err != nil || arrow == nil {
		return
	}
	if arrow.UserInstalled {
		return
	}

	parents, err := u.graph.GetDependents(ctx, ns)
	if err != nil {
		return
	}

	if countRunning(ctx, parents, u.runtime.GetState) > 0 {
		return
	}

	_ = u.runtime.BeginUninstall(ctx, ns, nil)
}

func (u *runtimeUsecase) onUninstallEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) { //nolint:gocyclo
	plan, err := u.graph.Resolve(ctx, rt.Ref)
	if err != nil {
		return
	}

	for _, entry := range plan {
		depNs := entry.Namespace
		state, stateErr := u.runtime.GetState(ctx, depNs)
		if stateErr != nil {
			continue
		}
		if state == domain.ArrowStateAbsent || state == domain.ArrowStateRemoved || state == "" {
			continue
		}

		parents, parentsErr := u.graph.GetDependents(ctx, depNs)
		if parentsErr != nil {
			slog.ErrorContext(ctx, "onUninstallEnded: GetDependents failed", "ns", depNs, "err", parentsErr)
			continue
		}

		filteredParents := make([]domain.Namespace, 0, len(parents))
		for _, p := range parents {
			if p != rt.Ref {
				filteredParents = append(filteredParents, p)
			}
		}

		if countRunning(ctx, filteredParents, u.runtime.GetState) > 0 {
			continue
		}

		arrow, getErr := u.arrow.Get(ctx, depNs)
		if getErr != nil || arrow == nil || arrow.UserInstalled {
			continue
		}

		switch state {
		case domain.ArrowStateRunning, domain.ArrowStateStopping:
			_ = u.runtime.BeginStop(ctx, depNs)
		case domain.ArrowStateReady:
			_ = u.runtime.BeginUninstall(ctx, depNs, nil)
		case domain.ArrowStateAbsent,
			domain.ArrowStateInstalling,
			domain.ArrowStateUpdating,
			domain.ArrowStateDraining,
			domain.ArrowStateDetached,
			domain.ArrowStateUninstalling,
			domain.ArrowStateRemoved,
			domain.ArrowStateOutdated:
		}
	}
}

func countRunning(
	ctx context.Context,
	nss []domain.Namespace,
	getState func(
		context.Context,
		domain.Namespace,
	) (domain.ArrowState, error),
) int {
	n := 0
	for _, ns := range nss {
		s, _ := getState(ctx, ns)
		if s != domain.ArrowStateAbsent && s != domain.ArrowStateRemoved && s != "" {
			n++
		}
	}
	return n
}
