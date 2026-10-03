package activation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// Arrow is what activating needs from the arrow catalog.
type Arrow interface {
	ResolveCatalogued(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.Namespace, error)
}

// Runtime is what activating needs from the runtime repository.
type Runtime interface {
	GetRuntime(
		ctx context.Context,
		ns domain.Namespace,
	) (*domainRuntime.ArrowRuntime, error)
	MarkActivating(
		ctx context.Context,
		ns domain.Namespace,
	) error
	ClearPendingActivation(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

// Trigger hands the daemon over to a binary. Fire asks the daemon to shut
// down gracefully and relaunch into path.
type Trigger interface {
	Fire(path string)
	Fired() bool
}

// Activator applies a staged activation.
type Activator interface {
	// Activate hands the daemon over to the binary staged for ns and reports
	// whether it did: false when nothing is staged, or a handover is already
	// under way.
	Activate(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
}

type activator struct {
	arrow   Arrow
	runtime Runtime
	trigger Trigger
}

// NewActivator builds an Activator. A nil trigger is a process that cannot
// hand itself over, such as any command that is not the daemon.
func NewActivator(
	arrow Arrow,
	runtime Runtime,
	trigger Trigger,
) Activator {
	return &activator{arrow: arrow, runtime: runtime, trigger: trigger}
}

func (a *activator) Activate(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	ns, err := a.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("activate: %w", err)
	}
	rt, err := a.runtime.GetRuntime(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("activate: read runtime of %s: %w", ns, err)
	}
	if rt == nil || rt.PendingActivation == nil || rt.PendingActivation.Activating {
		return false, nil
	}
	if a.trigger == nil {
		return false, fmt.Errorf("activate: %s: this process cannot restart itself: %w", ns, apperrors.ErrStateViolation)
	}
	if a.trigger.Fired() {
		return false, nil
	}

	pending := rt.PendingActivation
	if err := selfupdate.Verify(ctx, pending.Path, pending.Size, pending.Digest); err != nil {
		a.discard(ctx, ns, pending)
		return false, fmt.Errorf("activate: %s: staged binary discarded: %w: %w", ns, apperrors.ErrStateViolation, err)
	}

	if err := a.runtime.MarkActivating(ctx, ns); err != nil {
		if errors.Is(err, apperrors.ErrStateViolation) {
			return false, nil
		}
		return false, fmt.Errorf("activate: %w", err)
	}
	a.trigger.Fire(pending.Path)
	return true, nil
}

// discard drops a staged binary that is no longer the one that was verified,
// so it is never offered again.
func (a *activator) discard(
	ctx context.Context,
	ns domain.Namespace,
	pending *domainRuntime.PendingActivation,
) {
	if err := a.runtime.ClearPendingActivation(ctx, ns); err != nil {
		slog.WarnContext(ctx, "activate: drop the record of an unusable staged binary", "ns", ns, "err", err)
	}
	if err := fns.Remove(ctx, pending.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.WarnContext(ctx, "activate: remove an unusable staged binary", "ns", ns, "path", pending.Path, "err", err)
	}
}
