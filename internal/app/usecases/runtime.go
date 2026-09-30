package usecases

import (
	"context"
	"fmt"
	"log/slog"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowrepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle"
	runtimerepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

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
	// yet. See lifecycle.Lifecycle.Settling.
	Settling(ns domain.Namespace) bool
	Start(ctx context.Context)
	// Drain waits for every update commit in flight. See
	// lifecycle.Lifecycle.Drain.
	Drain(ctx context.Context) error
}

type runtimeUsecase struct {
	arrow     arrowrepo.Arrow
	runtime   runtimerepo.Runtime
	lifecycle lifecycle.Lifecycle
}

func NewRuntimeUsecase(
	arrow arrowrepo.Arrow,
	runtime runtimerepo.Runtime,
	lc lifecycle.Lifecycle,
) RuntimeUsecase {
	return &runtimeUsecase{
		arrow:     arrow,
		runtime:   runtime,
		lifecycle: lc,
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
	return u.lifecycle.Install(ctx, ns, userVars)
}

func (u *runtimeUsecase) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) error {
	if err := rejectReservedVariables(userVars); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	return u.lifecycle.Uninstall(ctx, ns, userVars)
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
	return u.lifecycle.Execute(ctx, ns, method, userVars)
}

func (u *runtimeUsecase) Update(
	ctx context.Context,
	ns domain.Namespace,
	userVars map[string]string,
) (bool, error) {
	if err := rejectReservedVariables(userVars); err != nil {
		return false, fmt.Errorf("update: %w", err)
	}
	return u.lifecycle.Update(ctx, ns, userVars)
}

func (u *runtimeUsecase) Stop(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return u.lifecycle.Stop(ctx, ns)
}

func (u *runtimeUsecase) Reset(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return u.lifecycle.Reset(ctx, ns)
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

func (u *runtimeUsecase) Settling(ns domain.Namespace) bool {
	return u.lifecycle.Settling(ns)
}

func (u *runtimeUsecase) Drain(ctx context.Context) error {
	return u.lifecycle.Drain(ctx)
}
