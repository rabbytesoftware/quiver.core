package arrow

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

// PreinstalledProbeFn runs an arrow's preinstalled lifecycle steps and returns
// nil only when every one of them succeeded — that is, when the software the
// arrow describes is already present on this machine by some means Quiver did
// not install it with. Any error means "not detected"; it is a normal answer,
// not a failure of the add.
type PreinstalledProbeFn func(
	ctx context.Context,
	ns domain.Namespace,
	steps domainStep.StepList,
	vars map[string]string,
) error

// MarkPreinstalledFn lands ns's runtime aggregate at Ready without an install
// ever having run. See runtime.MarkPreinstalled for the implementation the
// container wires in.
type MarkPreinstalledFn func(
	ctx context.Context,
	ns domain.Namespace,
) error

// ForgetRuntimeFn clears ns's runtime aggregate entirely. See
// runtime.ForgetPreinstalled and markIfPreinstalled's negative-probe branch,
// which uses it to clear an orphan Ready runtime a prior, aborted Add left
// behind.
type ForgetRuntimeFn func(
	ctx context.Context,
	ns domain.Namespace,
) error

type preinstalledOpts struct {
	os     domain.OS
	probe  PreinstalledProbeFn
	mark   MarkPreinstalledFn
	forget ForgetRuntimeFn
}

// enabled reports whether Add should run a preinstalled probe. All three
// funcs are required -- a half-wired container does nothing rather than
// producing the wrong catalog state.
func (o preinstalledOpts) enabled() bool {
	return o.probe != nil && o.mark != nil && o.forget != nil
}

type options struct {
	preinstalled preinstalledOpts
	// versionOutdatedSync is zero unless WithVersionOutdatedSync was passed;
	// see version_outdated.go.
	versionOutdatedSync SetVersionOutdatedFn
}

// Option configures New.
type Option func(*options)

// WithPreinstalledDetection enables Add-time preinstalled detection for
// manifests that declare a preinstalled lifecycle block for os. probe, mark
// and forget are taken together since none of the three is usable alone.
func WithPreinstalledDetection(
	os domain.OS,
	probe PreinstalledProbeFn,
	mark MarkPreinstalledFn,
	forget ForgetRuntimeFn,
) Option {
	return func(o *options) {
		o.preinstalled = preinstalledOpts{os: os, probe: probe, mark: mark, forget: forget}
	}
}

func resolveOptions(
	opts []Option,
) options {
	var o options
	for _, apply := range opts {
		apply(&o)
	}

	return o
}

// markIfPreinstalled runs ns's preinstalled lifecycle and, on a detection,
// lands its runtime at Ready -- before the catalog row exists at all. The
// ordering is the entire invariant: nothing can read the arrow until the
// catalog row is written, so a runtime marked Ready first can never be
// observed alongside an Absent one.
//
// The negative-probe branch clears any orphan Ready runtime a prior, aborted
// Add left behind -- the one place such an orphan can still be observed,
// since the row it belongs to was never written.
func (s *arrowService) markIfPreinstalled(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
) error {
	steps := s.preinstalledSteps(arrow)
	if len(steps) == 0 {
		return nil
	}

	catalogued, err := s.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("add: preinstalled check %s: %w", ns, err)
	}
	if catalogued {
		return nil
	}

	if probeErr := s.preinstalled.probe(ctx, ns, steps, preinstalledVars(ns, arrow, s.preinstalled.os)); probeErr != nil {
		slog.InfoContext(ctx, "add: preinstalled check found nothing", "ns", ns, "err", probeErr)
		if err := s.preinstalled.forget(ctx, ns); err != nil {
			return fmt.Errorf("add: preinstalled check %s: clear orphan runtime: %w", ns, err)
		}
		return nil
	}

	if err := s.preinstalled.mark(ctx, ns); err != nil {
		return fmt.Errorf("add: %w", err)
	}

	return nil
}

// preinstalledSteps returns the steps Add should probe with, or nothing at all
// when this arrow is none of its business: detection not wired, or no
// preinstalled block declared for the platform this machine is.
func (s *arrowService) preinstalledSteps(
	arrow *domain.Arrow,
) domainStep.StepList {
	if !s.preinstalled.enabled() || arrow == nil {
		return nil
	}

	target, ok := arrow.Targets[s.preinstalled.os]
	if !ok {
		return nil
	}

	return target.Lifecycle.Preinstalled
}

// preinstalledVars is the variable set a preinstalled probe is expanded
// against -- deliberately only what Add can compute before any aggregate,
// execution or workdir exists for this namespace. Built-ins go in before
// manifest defaults, matching the runtime assembler's own layering.
func preinstalledVars(
	ns domain.Namespace,
	arrow *domain.Arrow,
	os domain.OS,
) map[string]string {
	vars := map[string]string{
		domain.VarArrowNamespace: ns.String(),
		domain.VarPlatform:       os.String(),
		domain.VarRef:            ns.Ref(),
	}

	for _, v := range arrow.Variables {
		if v.Default != "" {
			vars[v.Name] = v.Default
		}
	}

	return vars
}
