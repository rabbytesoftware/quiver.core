package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowrepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	runtimerepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/deptree"
)

// updateCommitTimeout bounds closing an update bracket: one live ref listing
// and one manifest fetch, each already bounded by the manifold's own fetch
// timeout.
const updateCommitTimeout = 2 * time.Minute

type RuntimeUsecase interface {
	Install(
		ctx context.Context,
		ns domain.Namespace,
		userVars map[string]string,
	) (bool, error)
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
	// Update updates ns to what is ahead of it and reports whether an update
	// started: false when nothing is newer, an idempotent no-op no runtime
	// event will follow.
	Update(
		ctx context.Context,
		ns domain.Namespace,
		userVars map[string]string,
	) (bool, error)
	Stop(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Reset(
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
	// Settling reports whether an update of ns began and has not committed
	// yet. Its runtime may already read ready or outdated: the commit that
	// advances the row runs after the update's steps ended.
	Settling(ns domain.Namespace) bool
	Start(ctx context.Context)
	// Drain waits for every update commit in flight and refuses new ones,
	// so a shutdown never closes the stores under a commit. When ctx ends
	// first it aborts them and reports why: those rows stay outdated and
	// their next update runs again.
	Drain(ctx context.Context) error
}

type runtimeUsecase struct {
	arrow   arrowrepo.Arrow
	runtime runtimerepo.Runtime
	graph   graph.Graph
	targets *updateTargets
	commits *updateCommits
	holds   *badgeHolds
	detach  func(fn func())
	// commitTimeout bounds a detached update commit, which has no caller
	// whose context would end it.
	commitTimeout time.Duration
}

func NewRuntimeUsecase(
	arrow arrowrepo.Arrow,
	runtime runtimerepo.Runtime,
	graph graph.Graph,
) RuntimeUsecase {
	return newRuntimeUsecase(arrow, runtime, graph)
}

func newRuntimeUsecase(
	arrow arrowrepo.Arrow,
	runtime runtimerepo.Runtime,
	graph graph.Graph,
) *runtimeUsecase {
	return &runtimeUsecase{
		arrow:         arrow,
		runtime:       runtime,
		graph:         graph,
		targets:       newUpdateTargets(),
		commits:       newUpdateCommits(),
		holds:         &badgeHolds{dirty: map[domain.Namespace]bool{}},
		detach:        func(fn func()) { go fn() },
		commitTimeout: updateCommitTimeout,
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

func (u *runtimeUsecase) Install(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) (bool, error) {
	if err := rejectReservedVariables(userVars); err != nil {
		return false, fmt.Errorf("install: %w", err)
	}

	ns, err := u.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("install: %w", err)
	}

	exists, err := u.arrow.Exists(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("install: %w", err)
	}
	if !exists {
		return false, fmt.Errorf("install: %w", apperrors.ErrNotFound)
	}

	state, err := u.runtime.GetState(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("install: get state: %w", err)
	}
	if !needsInstall(state) {
		return false, u.installDeps(ctx, ns)
	}

	closeBracket, err := u.targets.open(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("install: %w", err)
	}
	defer closeBracket()
	if err := u.installDeps(ctx, ns); err != nil {
		return false, err
	}
	began, err := u.beginInstallHeld(ctx, ns, userVars)
	if err != nil {
		return false, fmt.Errorf("install: %w", err)
	}
	return began, nil
}

func (u *runtimeUsecase) installDeps(
	ctx context.Context,
	ns domain.Namespace,
) error {
	plan, err := u.graph.Resolve(ctx, ns)
	if err != nil {
		return fmt.Errorf("install: resolve deps: %w", err)
	}
	if err := u.installPlan(ctx, plan); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	return nil
}

func needsInstall(
	state domain.ArrowState,
) bool {
	return state == "" ||
		state == domain.ArrowStateAbsent ||
		state == domain.ArrowStateInstalling ||
		state == domain.ArrowStateRemoved
}

// beginInstall begins ns's install unless its runtime already has one, and
// reports whether it tried. Only a row that still needs an install takes its
// bracket, so an update installing its dependencies never waits on the
// bracket of an installed row, which may be its own or another update's.
func (u *runtimeUsecase) beginInstall(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) (bool, error) {
	state, err := u.runtime.GetState(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("get state: %w", err)
	}
	if !needsInstall(state) {
		return false, nil
	}

	closeBracket, err := u.targets.open(ctx, ns)
	if err != nil {
		return false, err
	}
	defer closeBracket()
	return u.beginInstallHeld(ctx, ns, vars)
}

// beginInstallHeld reads the state again under ns's bracket, so a catalog
// advance of ns cannot land while the install is assembled from the row it
// is leaving.
func (u *runtimeUsecase) beginInstallHeld(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) (bool, error) {
	state, err := u.runtime.GetState(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("get state: %w", err)
	}
	if !needsInstall(state) {
		return false, nil
	}
	return true, u.runtime.BeginInstall(ctx, ns, vars)
}

// installPlan catalogues every dependency in plan before installing any, so
// a dependency that cannot be resolved fails the install before anything
// runs. A plan entry names the declaration; what gets installed is the row
// that declaration catalogues as.
func (u *runtimeUsecase) installPlan(
	ctx context.Context,
	plan graph.Plan,
) error {
	identities := make(map[domain.Namespace]domain.Namespace, len(plan))
	for _, entry := range plan {
		identity, err := u.ensureDependency(ctx, entry.Namespace)
		if err != nil {
			return err
		}
		identities[entry.Namespace] = identity
	}

	for _, entry := range plan {
		depNs := identities[entry.Namespace]
		if err := u.installOneDep(ctx, depNs); err != nil {
			return err
		}
		if entry.Type != domain.ServiceDep {
			continue
		}
		if err := u.startServiceDep(ctx, depNs); err != nil {
			return err
		}
	}
	return nil
}

// ensureDependency returns the catalogued row a dependency declaration
// installs, adding it when missing. A declaration with a selector is its own
// identity; a bare one is decided when it is first added.
func (u *runtimeUsecase) ensureDependency(
	ctx context.Context,
	declared domain.Namespace,
) (domain.Namespace, error) {
	catalogued, err := u.isCatalogued(ctx, declared)
	if err != nil {
		return "", err
	}
	if catalogued {
		return declared, nil
	}

	identity, err := u.arrow.AddDependency(ctx, declared)
	if err != nil {
		return "", fmt.Errorf("add dep %s: %w", declared, err)
	}
	return identity, nil
}

// ensureAddedDeps catalogues the dependencies a row gained and returns the
// identity each declaration installs. A row that gained itself is refused.
func (u *runtimeUsecase) ensureAddedDeps(
	ctx context.Context,
	ns domain.Namespace,
	added []domain.Namespace,
) (map[domain.Namespace]domain.Namespace, error) {
	identities := make(map[domain.Namespace]domain.Namespace, len(added))
	for _, depNs := range added {
		identity, err := u.ensureDependency(ctx, depNs)
		if err != nil {
			return nil, fmt.Errorf("sync deps: %w", err)
		}
		if identity == ns {
			return nil, fmt.Errorf("sync deps: %s depends on itself: %w", ns, apperrors.ErrInvalidManifest)
		}
		identities[depNs] = identity
	}
	return identities, nil
}

func (u *runtimeUsecase) installOneDep(ctx context.Context, depNs domain.Namespace) error {
	ch, unsub, err := u.runtime.ListenEnded(ctx, depNs)
	if err != nil {
		return fmt.Errorf("install dep %s: listen: %w", depNs, err)
	}
	defer unsub()

	tried, beErr := u.beginInstall(ctx, depNs, nil)
	if beErr != nil && !errors.Is(beErr, apperrors.ErrStateViolation) {
		return fmt.Errorf("install dep %s: %w", depNs, beErr)
	}
	if !tried {
		return nil
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

	ns, err := u.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
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

	ns, err := u.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	if method == domain.MethodUpdate {
		_, err := u.executeUpdate(ctx, ns, userVars)
		return err
	}
	return u.runtime.BeginExecution(ctx, ns, method, userVars)
}

func (u *runtimeUsecase) Update(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) (bool, error) {
	if err := rejectReservedVariables(userVars); err != nil {
		return false, fmt.Errorf("update: %w", err)
	}

	ns, err := u.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("update: %w", err)
	}
	return u.executeUpdate(ctx, ns, userVars)
}

// executeUpdate opens the update bracket: it re-resolves what is ahead of
// the row, stops it if it runs, stages the target's manifest so the target's
// own update steps run, installs any dependency the target gained, and
// begins the update, reporting whether it started one. onUpdateEnded closes
// the bracket. Brackets of one row are serialized up to BeginUpdate, so a
// second one finds the first running instead of staging its own target under
// it, and one that arrives while the first is still settling (its commit or
// restore not landed yet) is refused rather than running the steps again.
func (u *runtimeUsecase) executeUpdate(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) (bool, error) {
	closeBracket, err := u.targets.open(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("update: %w", err)
	}
	defer closeBracket()
	if u.Settling(ns) {
		return false, fmt.Errorf("update: previous update of %s not settled: %w", ns, apperrors.ErrStateViolation)
	}

	state, err := u.runtime.GetState(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("execute: get state: %w", err)
	}
	if state != domain.ArrowStateReady &&
		state != domain.ArrowStateOutdated &&
		state != domain.ArrowStateRunning {
		if err := u.runtime.BeginExecution(ctx, ns, domain.MethodUpdate, userVars); err != nil {
			return false, err
		}
		return true, nil
	}

	current, err := u.arrow.Get(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("update: get current: %w", err)
	}
	available, err := u.arrow.CheckAvailable(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("update: %w", err)
	}
	if available == nil {
		return false, nil
	}

	if err := stopIfRunning(ctx, u.runtime, ns); err != nil {
		return false, fmt.Errorf("update: stop: %w", err)
	}
	if err := u.stageAndBegin(ctx, ns, current, *available, userVars); err != nil {
		return false, err
	}
	return true, nil
}

// restoreTimeout bounds putting the installed manifest back after a bracket
// failed, which must happen even when the caller gave up.
const restoreTimeout = 30 * time.Second

// stageAndBegin stages the target's manifest, syncs the dependencies it
// changes and begins its update. Anything that fails once the target is
// staged restores the installed release's manifest before returning: no run
// began, so nothing would ever end to restore it, and the next install would
// run the target's recipe for the installed ${REF}. The restore is tracked
// like a commit, so a shutdown drain waits for it, aborts it, or refuses it.
func (u *runtimeUsecase) stageAndBegin(
	ctx context.Context,
	ns domain.Namespace,
	current *domain.Arrow,
	available domain.Available,
	userVars map[string]string,
) (err error) {
	target, err := u.arrow.RefreshToTarget(ctx, ns, available)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	began := false
	defer func() {
		if err == nil || began || !u.commits.begin(ns) {
			return
		}
		defer u.commits.done(ns)
		restoreCtx, cancel := u.commits.bound(context.WithoutCancel(ctx), restoreTimeout)
		defer cancel()
		u.restoreInstalled(restoreCtx, ns)
	}()

	if err := u.syncTargetDeps(ctx, ns, current, target); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	undo := u.remember(ns, available)
	if err := u.runtime.BeginUpdate(ctx, ns, userVars, available.Ref); err != nil {
		if began = ctx.Err() != nil && u.updateBegan(ctx, ns); !began {
			undo()
		}
		return err
	}
	return nil
}

// updateBegan reports, after the caller gave up during BeginUpdate, whether
// the update was accepted anyway: the runtime reads updating, or the run
// already ended and its end took the remembered target. A run that began
// owns the outcome, and its end settles the row.
func (u *runtimeUsecase) updateBegan(
	ctx context.Context,
	ns domain.Namespace,
) bool {
	if !u.targets.pending(ns) {
		return true
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreTimeout)
	defer cancel()
	state, err := u.runtime.GetState(readCtx, ns)
	return err == nil && state == domain.ArrowStateUpdating
}

// remember records the target an update begins toward, except for
// quiver.core's own row, whose end commits nothing from here.
func (u *runtimeUsecase) remember(
	ns domain.Namespace,
	target domain.Available,
) func() {
	if isSelfNamespace(ns) {
		return func() {}
	}
	return u.targets.put(ns, target)
}

// syncTargetDeps lands the dependencies the target manifest gained or lost
// as pending on the row, then syncs whatever is pending. A retried update
// finds its manifest already staged, so it syncs what the first attempt left
// pending rather than a diff that is now empty.
func (u *runtimeUsecase) syncTargetDeps(
	ctx context.Context,
	ns domain.Namespace,
	current *domain.Arrow,
	target *domain.Arrow,
) error {
	diff := u.graph.DiffDeps(current, target)
	if len(diff.Added) > 0 || len(diff.Removed) > 0 {
		if err := u.runtime.MarkOutdated(ctx, ns, edgesToNs(diff.Added), edgesToNs(diff.Removed)); err != nil {
			return fmt.Errorf("mark outdated: %w", err)
		}
	}

	state, err := u.runtime.GetState(ctx, ns)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}
	if state != domain.ArrowStateOutdated {
		return nil
	}
	return u.syncDeps(ctx, ns)
}

func (u *runtimeUsecase) Stop(
	ctx context.Context,
	ns domain.Namespace,
) error {
	ns, err := u.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return fmt.Errorf("stop: %w", err)
	}

	return u.runtime.BeginStop(ctx, ns)
}

// Reset forgets the runtime aggregate, clearing a runtime stuck in a transient
// state. The catalog entry (if any) is left intact so the arrow can be re-installed.
func (u *runtimeUsecase) Reset(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := u.runtime.Forget(ctx, ns); err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	u.targets.take(ns)
	return nil
}

func (u *runtimeUsecase) syncDeps(
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

	identities, err := u.ensureAddedDeps(ctx, ns, syncInfo.AddedDeps)
	if err != nil {
		return err
	}
	plan, err := u.graph.Resolve(ctx, ns)
	if errors.Is(err, deptree.ErrCyclicDependency) {
		return fmt.Errorf("sync deps: %w: %w", apperrors.ErrInvalidManifest, err)
	}

	for _, depNs := range syncInfo.AddedDeps {
		if err := u.installOneDep(ctx, identities[depNs]); err != nil {
			return err
		}
	}

	if err == nil { //nolint:nestif
		planMap := make(map[domain.Namespace]domain.DepType, len(plan))
		for _, entry := range plan {
			planMap[entry.Namespace] = entry.Type
		}
		for _, depNs := range syncInfo.AddedDeps {
			if planMap[depNs] == domain.ServiceDep {
				if startErr := u.startServiceDep(ctx, identities[depNs]); startErr != nil {
					return fmt.Errorf("sync deps: start service dep %s: %w", depNs, startErr)
				}
			}
		}
	}

	for _, declared := range syncInfo.RemovedDeps {
		u.retireRemovedDep(ctx, declared)
	}

	return nil
}

// retireRemovedDep uninstalls, or stops, a dependency a row no longer
// declares once nothing else needs it. The declaration is mapped onto the
// row the catalog holds it under, since it may spell a commit in another case.
func (u *runtimeUsecase) retireRemovedDep(
	ctx context.Context,
	declared domain.Namespace,
) {
	depNs, err := u.arrow.ResolveCatalogued(ctx, declared)
	if err != nil {
		return
	}
	arrow, getErr := u.arrow.Get(ctx, depNs)
	if getErr != nil || arrow == nil || arrow.UserInstalled {
		return
	}
	hasDeps, depsErr := u.graph.HasDependents(ctx, depNs, "")
	if depsErr != nil || hasDeps {
		return
	}
	depState, stateErr := u.runtime.GetState(ctx, depNs)
	if stateErr != nil {
		return
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
	ns, err := u.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get runtime: %w", err)
	}

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

// ListRuntimes reports one runtime per installed ref.
//
// A catalog view is keyed by the bare arrow identity and its refs live in
// Versions, while runtime aggregates are keyed by the versioned namespace.
// Reading by the bare namespace matches nothing, so every arrow came back as
// the synthesized "absent" and `quiver ps` could never show a running arrow.
func (u *runtimeUsecase) ListRuntimes(
	ctx context.Context,
) ([]domainRuntime.ArrowRuntime, error) {
	views, err := u.arrow.List(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list runtimes: list arrows: %w", err)
	}

	runtimes := make([]domainRuntime.ArrowRuntime, 0, len(views))

	for _, v := range views {
		for _, ver := range v.Versions {
			runtimes = append(runtimes, u.runtimeOrAbsent(ctx, ver.Namespace))
		}
	}

	return runtimes, nil
}

// runtimeOrAbsent reads one arrow's runtime, reporting absent when there is no
// aggregate yet or it cannot be read.
//
// One unreadable aggregate must not fail the whole listing: this backs
// `quiver ps`, the command a user reaches for precisely when something has
// already gone wrong.
func (u *runtimeUsecase) runtimeOrAbsent(
	ctx context.Context,
	ns domain.Namespace,
) domainRuntime.ArrowRuntime {
	rt, err := u.runtime.GetRuntime(ctx, ns)
	if err != nil {
		slog.ErrorContext(ctx, "list runtimes: read runtime", "ns", ns, "err", err)
	}

	if rt == nil {
		return domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateAbsent}
	}

	return *rt
}

func (u *runtimeUsecase) Start(ctx context.Context) {
	u.runtime.Start(ctx)
}

// Settling reads the remembered target before the running commits:
// onUpdateEnded registers the commit before it releases the target, so in
// this order no read falls between the two.
func (u *runtimeUsecase) Settling(ns domain.Namespace) bool {
	return u.targets.pending(ns) || u.commits.inFlight(ns)
}

func (u *runtimeUsecase) Drain(ctx context.Context) error {
	if err := u.commits.drain(ctx); err != nil {
		slog.WarnContext(ctx, "update: shutdown abandoned commits in flight; their rows stay outdated", "err", err)
		return fmt.Errorf("drain update commits: %w", err)
	}
	return nil
}

// onUpdateEnded closes the update bracket: it always releases the row's
// remembered target, so the next update is admitted, and settles the row:
// it commits the target once the update steps succeeded, and otherwise puts
// the installed release's manifest back on the row.
// quiver.core's own update is excluded: its relaunched binary adopts its new
// state on boot.
//
// The settling runs detached because this handler is delivered on the
// runtime aggregate's own ordered event queue, and clearing the badge waits
// for that same queue: done inline, it would wait for itself. Detached is not
// untracked: the row reads settling until it is done, and a shutdown drains
// it before closing the stores.
func (u *runtimeUsecase) onUpdateEnded(ctx context.Context, rt domainRuntime.ArrowRuntime) {
	admitted := u.commits.begin(rt.Ref)
	target, recorded := u.targets.take(rt.Ref)
	if !admitted {
		slog.WarnContext(ctx, "update: shutting down; the row stays outdated until its next update", "ns", rt.Ref)
		return
	}

	u.detach(func() {
		settleCtx, cancel := u.commits.bound(context.WithoutCancel(ctx), u.commitTimeout)
		defer cancel()
		defer u.releaseSettled(settleCtx, rt.Ref)
		u.settleUpdate(settleCtx, rt, target, recorded)
	})
}

// badgeHolds records the rows whose version check was held while they
// settled, so the settle re-derives their badge before it lets them go.
type badgeHolds struct {
	mu    sync.Mutex
	dirty map[domain.Namespace]bool
}

// HoldBadge tells a version check whether to leave ns's badge alone: while
// ns settles, the row still names the target about to be stamped. A held
// check is remembered, and releaseSettled re-derives the badge for it.
func (u *runtimeUsecase) HoldBadge(ns domain.Namespace) bool {
	u.holds.mu.Lock()
	defer u.holds.mu.Unlock()
	if !u.Settling(ns) {
		return false
	}
	u.holds.dirty[ns] = true
	return true
}

// releaseSettled re-derives ns's badge from the row and lets the row go. A
// check held before a reconcile is covered by it, since the reconcile reads
// the row afterwards; only a check held while the reconcile ran may have
// recorded something it did not read, and only then is it run again. Once
// the row is released, checks sync the badge themselves.
func (u *runtimeUsecase) releaseSettled(
	ctx context.Context,
	ns domain.Namespace,
) {
	for {
		u.holds.mu.Lock()
		delete(u.holds.dirty, ns)
		u.holds.mu.Unlock()

		u.reconcileBadge(ctx, ns)

		u.holds.mu.Lock()
		if !u.holds.dirty[ns] {
			u.commits.done(ns)
			u.holds.mu.Unlock()
			return
		}
		u.holds.mu.Unlock()
	}
}

// settleUpdate commits a succeeded update, or restores the installed
// release's manifest when nothing is stamped: a failed update, or one whose
// target moved, leaves the row reporting what it had installed, so the next
// install or execution runs that release's own steps for its own ${REF}.
func (u *runtimeUsecase) settleUpdate(
	ctx context.Context,
	rt domainRuntime.ArrowRuntime,
	target domain.Available,
	recorded bool,
) {
	succeeded := rt.LastReturn != nil && rt.LastReturn.Outcome == domainRuntime.ExecutionOutcomeSuccess
	if isSelfNamespace(rt.Ref) {
		if !succeeded {
			u.restoreInstalled(ctx, rt.Ref)
		}
		return
	}
	if !recorded {
		if succeeded {
			slog.WarnContext(ctx, "update: no target recorded for a finished update", "ns", rt.Ref)
		}
		return
	}
	if succeeded && u.commitUpdate(ctx, rt.Ref, target) {
		return
	}
	restoreCtx, cancel, ok := u.afterSettle(ctx)
	if !ok {
		return
	}
	defer cancel()
	u.restoreInstalled(restoreCtx, rt.Ref)
}

// afterSettle gives the writes that close a settling their own context once
// the settling's ran out: a commit that timed out still restores the row and
// reconciles its badge. That context is bounded like a commit's, so a drain
// that gives up aborts it too, and a settling a shutdown drain aborted writes
// nothing: the stores are about to close.
func (u *runtimeUsecase) afterSettle(
	ctx context.Context,
) (context.Context, context.CancelFunc, bool) {
	if ctx.Err() == nil {
		return ctx, func() {}, true
	}
	if u.commits.isDraining() {
		return nil, nil, false
	}
	fresh, cancel := u.commits.bound(context.WithoutCancel(ctx), restoreTimeout)
	return fresh, cancel, true
}

// commitUpdate stamps target as installed only if it is still what its ref
// names: when the target moved while its update ran, the installed bits are
// not the target's, so nothing is stamped and the row stays outdated. The
// worst case is an extra update, never a missed one. It reports whether the
// row advanced.
func (u *runtimeUsecase) commitUpdate(
	ctx context.Context,
	ns domain.Namespace,
	target domain.Available,
) bool {
	unmoved, err := u.arrow.TargetUnmoved(ctx, ns, target)
	if err != nil {
		slog.WarnContext(ctx, "update: re-resolve target", "ns", ns, "err", err)
		return false
	}
	if !unmoved {
		slog.WarnContext(ctx, "update: target moved during update",
			"ns", ns, "ref", target.Ref, "commit", target.Commit)
		return false
	}

	if err := u.arrow.Advance(ctx, ns, target); err != nil {
		slog.ErrorContext(ctx, "update: advance", "ns", ns, "err", err)
		return false
	}
	return true
}

// reconcileBadge lands the runtime badge on the row as the settled update
// left it: Ready once nothing is available, Outdated while the row still
// has something ahead (a newer release recorded during the update, or a
// target that was not stamped). See afterSettle for a settling that ran out.
func (u *runtimeUsecase) reconcileBadge(
	ctx context.Context,
	ns domain.Namespace,
) {
	ctx, cancel, ok := u.afterSettle(ctx)
	if !ok {
		return
	}
	defer cancel()
	if err := u.runtime.ReconcileVersionBadge(ctx, ns); err != nil {
		slog.ErrorContext(ctx, "update: reconcile version badge", "ns", ns, "err", err)
	}
}

// restoreInstalled stages the manifest of the release the row has installed
// again, replacing the target manifest the bracket staged. A row that
// recorded no commit (one written before commits were recorded) has nothing
// to fetch it at and keeps the staged manifest until its next update.
func (u *runtimeUsecase) restoreInstalled(
	ctx context.Context,
	ns domain.Namespace,
) {
	row, err := u.arrow.Get(ctx, ns)
	if err != nil {
		slog.WarnContext(ctx, "update: read row to restore its installed manifest", "ns", ns, "err", err)
		return
	}
	if row == nil || row.Resolved.Commit == "" {
		return
	}
	installed := domain.Available{Ref: row.Resolved.Ref, Commit: row.Resolved.Commit}
	if _, err := u.arrow.RefreshToTarget(ctx, ns, installed); err != nil {
		slog.WarnContext(ctx, "update: restore the installed manifest", "ns", ns, "err", err)
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
	case domain.MethodUpdate:
		u.onUpdateEnded(ctx, rt)
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
		depNs, catErr := u.arrow.ResolveCatalogued(ctx, entry.Namespace)
		if catErr != nil {
			continue
		}
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
		case domain.ArrowStateReady, domain.ArrowStateOutdated:
			_ = u.runtime.BeginUninstall(ctx, depNs, nil)
		case domain.ArrowStateAbsent,
			domain.ArrowStateInstalling,
			domain.ArrowStateUpdating,
			domain.ArrowStateDraining,
			domain.ArrowStateDetached,
			domain.ArrowStateUninstalling,
			domain.ArrowStateRemoved:
		}
	}
}

// stopIfRunning stops ns and waits for the stop to finish before returning,
// so an update never races a still-running execution. A no-op for any state
// other than Running.
func stopIfRunning(
	ctx context.Context,
	rt runtimerepo.Runtime,
	ns domain.Namespace,
) error {
	state, err := rt.GetState(ctx, ns)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}
	if state != domain.ArrowStateRunning {
		return nil
	}

	ch, unsub, err := rt.ListenEnded(ctx, ns)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer unsub()

	if err := rt.BeginStop(ctx, ns); err != nil {
		return fmt.Errorf("begin stop: %w", err)
	}

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// isCatalogued reports whether declared is already a row; a bare
// declaration never is, since no row is keyed without a selector.
func (u *runtimeUsecase) isCatalogued(
	ctx context.Context,
	declared domain.Namespace,
) (bool, error) {
	if declared.Ref() == "" {
		return false, nil
	}
	exists, err := u.arrow.Exists(ctx, declared)
	if err != nil {
		return false, fmt.Errorf("check dep %s: %w", declared, err)
	}
	return exists, nil
}

// isSelfNamespace reports whether ns is a ref of quiver.core's own self-arrow
// namespace; the "@" matters, or any namespace merely starting with it would match.
func isSelfNamespace(ns domain.Namespace) bool {
	self, _ := metadata.GetSelfNamespaces()
	return strings.HasPrefix(ns.String(), string(self)+"@")
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
