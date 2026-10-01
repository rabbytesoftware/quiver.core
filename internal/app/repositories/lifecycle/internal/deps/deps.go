package deps

import (
	"context"
	"fmt"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// Deps runs the verbs that walk a row's dependencies and the cascades that
// follow a runtime's end.
type Deps interface {
	Install(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) (bool, error)
	Uninstall(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) error
	Stop(
		ctx context.Context,
		ns domain.Namespace,
	) error
	// StopIfRunning stops ns and waits for the stop to finish before
	// returning, so an update never races a still-running execution. A no-op
	// for any state other than Running.
	StopIfRunning(
		ctx context.Context,
		ns domain.Namespace,
	) error
	// SyncTargetDeps lands the dependencies the target manifest gained or
	// lost as pending on the row, then syncs whatever is pending.
	SyncTargetDeps(
		ctx context.Context,
		ns domain.Namespace,
		current *domain.Arrow,
		target *domain.Arrow,
	) error
	OnRuntimeEnded(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	)
}

type deps struct {
	arrow       Arrow
	runtime     Runtime
	graph       Graph
	brackets    Brackets
	updateEnded func(ctx context.Context, rt domainRuntime.ArrowRuntime)
}

func New(
	arrow Arrow,
	runtime Runtime,
	graph Graph,
	brackets Brackets,
	updateEnded func(ctx context.Context, rt domainRuntime.ArrowRuntime),
) Deps {
	return &deps{
		arrow:       arrow,
		runtime:     runtime,
		graph:       graph,
		brackets:    brackets,
		updateEnded: updateEnded,
	}
}

func (d *deps) Install(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) (bool, error) {
	ns, err := d.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("install: %w", err)
	}

	exists, err := d.arrow.Exists(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("install: %w", err)
	}
	if !exists {
		return false, fmt.Errorf("install: %w", apperrors.ErrNotFound)
	}

	state, err := d.runtime.GetState(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("install: get state: %w", err)
	}
	if !needsInstall(state) {
		return false, d.installDeps(ctx, ns)
	}

	closeBracket, err := d.brackets.Open(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("install: %w", err)
	}
	defer closeBracket()
	if err := d.advanceToAvailable(ctx, ns); err != nil {
		return false, fmt.Errorf("install: %w", err)
	}
	if err := d.installDeps(ctx, ns); err != nil {
		return false, err
	}
	began, err := d.beginInstallHeld(ctx, ns, vars)
	if err != nil {
		return false, fmt.Errorf("install: %w", err)
	}
	return began, nil
}

func (d *deps) Uninstall(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) error {
	ns, err := d.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}

	hasDeps, err := d.graph.HasDependents(ctx, ns, "")
	if err != nil {
		return err
	}
	if hasDeps {
		return fmt.Errorf("uninstall: %w", apperrors.ErrDependentsExist)
	}
	return d.runtime.BeginUninstall(ctx, ns, vars)
}

func (d *deps) Stop(
	ctx context.Context,
	ns domain.Namespace,
) error {
	ns, err := d.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return fmt.Errorf("stop: %w", err)
	}

	return d.runtime.BeginStop(ctx, ns)
}

func (d *deps) StopIfRunning(
	ctx context.Context,
	ns domain.Namespace,
) error {
	state, err := d.runtime.GetState(ctx, ns)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}
	if state != domain.ArrowStateRunning {
		return nil
	}

	ch, unsub, err := d.runtime.ListenEnded(ctx, ns)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer unsub()

	if err := d.runtime.BeginStop(ctx, ns); err != nil {
		return fmt.Errorf("begin stop: %w", err)
	}

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
