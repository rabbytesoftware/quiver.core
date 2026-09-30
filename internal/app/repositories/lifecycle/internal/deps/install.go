package deps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func (d *deps) installDeps(
	ctx context.Context,
	ns domain.Namespace,
) error {
	plan, err := d.graph.Resolve(ctx, ns)
	if err != nil {
		return fmt.Errorf("install: resolve deps: %w", err)
	}
	if err := d.installPlan(ctx, plan); err != nil {
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
func (d *deps) beginInstall(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) (bool, error) {
	state, err := d.runtime.GetState(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("get state: %w", err)
	}
	if !needsInstall(state) {
		return false, nil
	}

	closeBracket, err := d.brackets.Open(ctx, ns)
	if err != nil {
		return false, err
	}
	defer closeBracket()
	return d.beginInstallHeld(ctx, ns, vars)
}

// beginInstallHeld reads the state again under ns's bracket, so a catalog
// advance of ns cannot land while the install is assembled from the row it
// is leaving.
func (d *deps) beginInstallHeld(
	ctx context.Context,
	ns domain.Namespace,
	vars map[string]string,
) (bool, error) {
	state, err := d.runtime.GetState(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("get state: %w", err)
	}
	if !needsInstall(state) {
		return false, nil
	}
	return true, d.runtime.BeginInstall(ctx, ns, vars)
}

// installPlan catalogues every dependency in plan before installing any, so
// a dependency that cannot be resolved fails the install before anything
// runs. A plan entry names the declaration; what gets installed is the row
// that declaration catalogues as.
func (d *deps) installPlan(
	ctx context.Context,
	plan models.Plan,
) error {
	identities := make(map[domain.Namespace]domain.Namespace, len(plan))
	for _, entry := range plan {
		identity, err := d.ensureDependency(ctx, entry.Namespace)
		if err != nil {
			return err
		}
		identities[entry.Namespace] = identity
	}

	for _, entry := range plan {
		depNs := identities[entry.Namespace]
		if err := d.installOneDep(ctx, depNs); err != nil {
			return err
		}
		if entry.Type != domain.ServiceDep {
			continue
		}
		if err := d.startServiceDep(ctx, depNs); err != nil {
			return err
		}
	}
	return nil
}

// ensureDependency returns the catalogued row a dependency declaration
// installs, adding it when missing. A declaration with a selector is its own
// identity; a bare one is decided when it is first added.
func (d *deps) ensureDependency(
	ctx context.Context,
	declared domain.Namespace,
) (domain.Namespace, error) {
	catalogued, err := d.isCatalogued(ctx, declared)
	if err != nil {
		return "", err
	}
	if catalogued {
		return declared, nil
	}

	identity, err := d.arrow.AddDependency(ctx, declared)
	if err != nil {
		return "", fmt.Errorf("add dep %s: %w", declared, err)
	}
	return identity, nil
}

func (d *deps) installOneDep(ctx context.Context, depNs domain.Namespace) error {
	ch, unsub, err := d.runtime.ListenEnded(ctx, depNs)
	if err != nil {
		return fmt.Errorf("install dep %s: listen: %w", depNs, err)
	}
	defer unsub()

	tried, beErr := d.beginInstall(ctx, depNs, nil)
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

func (d *deps) startServiceDep(ctx context.Context, depNs domain.Namespace) error {
	state, err := d.runtime.GetState(ctx, depNs)
	if err != nil {
		return fmt.Errorf("start service dep %s: get state: %w", depNs, err)
	}
	if state == domain.ArrowStateRunning {
		return nil
	}

	if beErr := d.runtime.BeginExecution(ctx, depNs, domain.MethodExecute, nil); beErr != nil {
		if !errors.Is(beErr, apperrors.ErrStateViolation) {
			return fmt.Errorf("start service dep %s: %w", depNs, beErr)
		}
	}
	return nil
}

// isCatalogued reports whether declared is already a row; a bare
// declaration never is, since no row is keyed without a selector.
func (d *deps) isCatalogued(
	ctx context.Context,
	declared domain.Namespace,
) (bool, error) {
	if declared.Ref() == "" {
		return false, nil
	}
	exists, err := d.arrow.Exists(ctx, declared)
	if err != nil {
		return false, fmt.Errorf("check dep %s: %w", declared, err)
	}
	return exists, nil
}
