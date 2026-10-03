package bracket

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// restoreTimeout bounds reading a row back after its caller gave up, which
// must happen even then.
const restoreTimeout = 30 * time.Second

// Updater opens update brackets and runs the verbs that go through one.
type Updater interface {
	Execute(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		vars map[string]string,
	) error
	// Update updates ns to what is ahead of it and reports whether an update
	// started: false when nothing is newer, an idempotent no-op no runtime
	// event will follow.
	Update(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) (bool, error)
	// Reset forgets the runtime aggregate, clearing a runtime stuck in a
	// transient state, and the target an update of it remembered. The
	// catalog entry is left intact so the arrow can be re-installed.
	Reset(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

type updater struct {
	arrow   Arrow
	runtime Runtime
	targets Targets
	settler Settler
	deps    Deps
}

func NewUpdater(
	arrow Arrow,
	runtime Runtime,
	targets Targets,
	settler Settler,
	deps Deps,
) Updater {
	return &updater{
		arrow:   arrow,
		runtime: runtime,
		targets: targets,
		settler: settler,
		deps:    deps,
	}
}

func (u *updater) Execute(
	ctx context.Context,
	ns domain.Namespace,
	method string,
	vars map[string]string,
) error {
	ns, err := u.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	if method == domain.MethodUpdate {
		_, err := u.executeUpdate(ctx, ns, vars)
		return err
	}
	return u.runtime.BeginExecution(ctx, ns, method, vars)
}

func (u *updater) Update(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) (bool, error) {
	ns, err := u.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("update: %w", err)
	}
	return u.executeUpdate(ctx, ns, vars)
}

func (u *updater) Reset(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := u.runtime.Forget(ctx, ns); err != nil {
		return fmt.Errorf("reset: %w", err)
	}
	u.targets.Take(ns)
	return nil
}

// executeUpdate opens the update bracket: it re-resolves what is ahead of
// the row, stops it if it runs, stages the target's manifest so the target's
// own update steps run, installs any dependency the target gained, and
// begins the update, reporting whether it started one. The update's end
// closes the bracket. Brackets of one row are serialized up to BeginUpdate,
// so a second one finds the first running instead of staging its own target
// under it, and one that arrives while the first is still settling (its
// commit or restore not landed yet) is refused rather than running the steps
// again.
func (u *updater) executeUpdate(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) (bool, error) {
	closeBracket, err := u.targets.Open(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("update: %w", err)
	}
	defer closeBracket()
	if u.settler.Settling(ns) {
		return false, fmt.Errorf("update: previous update of %s not settled: %w", ns, apperrors.ErrStateViolation)
	}

	state, err := u.runtime.GetState(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("execute: get state: %w", err)
	}
	if state != domain.ArrowStateReady &&
		state != domain.ArrowStateOutdated &&
		state != domain.ArrowStateRunning {
		if err := u.runtime.BeginExecution(ctx, ns, domain.MethodUpdate, vars); err != nil {
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
	if available == nil || u.alreadyStaged(ctx, ns, *available) {
		return false, nil
	}

	if err := u.deps.StopIfRunning(ctx, ns); err != nil {
		return false, fmt.Errorf("update: stop: %w", err)
	}
	if err := u.stageAndBegin(ctx, ns, current, *available, vars); err != nil {
		return false, err
	}
	return true, nil
}

// alreadyStaged reports whether the binary waiting to be applied is the
// target's own, so running the update again would fetch it a second time. A
// runtime that cannot be read says nothing about it.
func (u *updater) alreadyStaged(
	ctx context.Context,
	ns domain.Namespace,
	target domain.Available,
) bool {
	rt, err := u.runtime.GetRuntime(ctx, ns)
	if err != nil {
		slog.WarnContext(ctx, "update: read runtime for a staged activation", "ns", ns, "err", err)
		return false
	}
	if rt == nil || rt.PendingActivation == nil {
		return false
	}
	pending := rt.PendingActivation
	return pending.Version == target.Ref && (pending.Commit == "" || pending.Commit == target.Commit)
}

// stageAndBegin stages the target's manifest, syncs the dependencies it
// changes and begins its update. Anything that fails once the target is
// staged restores the installed release's manifest before returning: no run
// began, so nothing would ever end to restore it, and the next install would
// run the target's recipe for the installed ${REF}.
func (u *updater) stageAndBegin(
	ctx context.Context,
	ns domain.Namespace,
	current *domain.Arrow,
	available domain.Available,
	vars map[string]string,
) (err error) {
	target, err := u.arrow.RefreshToTarget(ctx, ns, available)
	if err != nil {
		u.rejudge(ctx, ns)
		return fmt.Errorf("update: %w", err)
	}
	began := false
	defer func() {
		if err == nil || began {
			return
		}
		u.settler.RestoreAbandoned(ctx, ns)
	}()

	if err := u.deps.SyncTargetDeps(ctx, ns, current, target); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	undo := u.targets.Put(ns, available)
	if err := u.runtime.BeginUpdate(ctx, ns, vars, available.Ref); err != nil {
		if began = ctx.Err() != nil && u.updateBegan(ctx, ns); !began {
			undo()
		}
		return err
	}
	return nil
}

// rejudge checks ns again after its target could not be staged: a target the
// fetch found empty is recorded so, and stops being offered.
func (u *updater) rejudge(
	ctx context.Context,
	ns domain.Namespace,
) {
	if _, err := u.arrow.CheckAvailable(ctx, ns); err != nil {
		slog.WarnContext(ctx, "update: judge the target again", "ns", ns, "err", err)
	}
}

// updateBegan reports, after the caller gave up during BeginUpdate, whether
// the update was accepted anyway: the runtime reads updating, or the run
// already ended and its end took the remembered target. A run that began owns
// the outcome, and its end settles the row.
func (u *updater) updateBegan(
	ctx context.Context,
	ns domain.Namespace,
) bool {
	if !u.targets.Pending(ns) {
		return true
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), restoreTimeout)
	defer cancel()
	state, err := u.runtime.GetState(readCtx, ns)
	return err == nil && state == domain.ArrowStateUpdating
}
