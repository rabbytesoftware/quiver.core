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
// runtime.ForgetPreinstalled for the implementation the container wires in,
// and markIfPreinstalled's negative-probe branch for the one case that needs
// it: a prior Add detected something and marked the runtime Ready, then died
// before its catalog row was ever written, and this Add's own probe finds
// nothing.
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

// enabled reports whether Add should consider running a preinstalled probe at
// all. All three are required: detecting an arrow without being able to mark
// it Ready, or without being able to clear a stale orphan runtime a negative
// probe finds, would each produce exactly the wrong catalog state this
// mechanism exists to prevent — so a half-wired container does nothing rather
// than something wrong.
func (o preinstalledOpts) enabled() bool {
	return o.probe != nil && o.mark != nil && o.forget != nil
}

type options struct {
	preinstalled preinstalledOpts
}

// Option configures New.
type Option func(*options)

// WithPreinstalledDetection enables Add-time preinstalled detection for
// manifests that declare a preinstalled lifecycle block for os. Without it —
// and every existing caller is without it — Add behaves exactly as it always
// has, for every arrow.
//
// probe, mark and forget are taken together because none of the three is
// usable alone.
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
// lands its runtime at Ready — before the catalog row exists at all.
//
// The ordering is the entire invariant. Nothing can read the arrow until
// addArrowCommand has run, so a runtime written first is a runtime that was
// already Ready for every caller that ever saw the arrow; there is no
// interleaving that exposes UserInstalled with an Absent runtime. Written the
// other way round that window is real, and no amount of promptness closes it.
//
// The cost of that ordering is the opposite failure: an add that dies after the
// runtime write leaves a Ready runtime with no catalog row. That orphan is not
// self-healing on its own: nothing in this codebase reconciles a runtime
// against a catalog row that was never written, so a later Add for the same
// namespace whose own probe finds nothing would otherwise inherit the earlier
// attempt's stale Ready — an arrow reported installed with no verified
// detection behind it, exactly what this mechanism exists to prevent. The
// negative-probe branch below closes that: it is the one place an orphan can
// still be observed (a namespace with no catalog row, about to get one), so it
// is the one place that clears it before returning.
//
// An arrow with no preinstalled block for this platform, or one already in the
// catalog, returns here having done nothing at all. The catalog test is what
// keeps a repeated announcement cheap and harmless — quiver.desktop re-adds
// itself on every boot, and a re-probe would both spawn a subprocess each time
// and be refused by an aggregate that has since moved past Ready.
//
// Any probe error is "not detected", including a wizard that is shutting down.
// That refusal is unreachable from a request in practice: the daemon drains the
// API before it shuts the app down (internal.Container.shutdownPhases), so an
// Add in flight finishes before the wizard stops accepting probes.
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
// against. It is deliberately only what Add can compute for a namespace that is
// not in the catalog yet: no aggregate exists to read stored variables from, no
// execution exists to allocate netbridge ports for, and no workdir exists
// either — nor would one help, since a check for software Quiver did not
// install has no use for the directory Quiver would have installed it into.
//
// Built-ins go in before manifest defaults, matching the layering the runtime
// assembler already resolves executions with.
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
