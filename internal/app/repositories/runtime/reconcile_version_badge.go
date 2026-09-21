package runtime

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// ReconcileVersionBadge returns the function the drain calls once an
// execution has ended, to put back the outdated badge EndExecution just
// dropped -- see reconcileVersionBadge in runtime/internal/hooks.go for why
// this re-derives from the catalog rather than checking the remote again.
//
// Reuses SetVersionOutdated so which transitions are legal stays the
// aggregate's own business. An arrow the catalog no longer holds -- forgotten
// while it ran -- has no answer to project, and the error says so.
func ReconcileVersionBadge(
	getArrow GetArrowFn,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
) func(ctx context.Context, ns domain.Namespace) error {
	setVersionOutdated := SetVersionOutdated(axRuntime)

	return func(ctx context.Context, ns domain.Namespace) error {
		arrow, err := getArrow(ctx, ns)
		if err != nil {
			return fmt.Errorf("reconcile version badge %s: %w", ns, err)
		}
		if arrow == nil {
			return nil
		}

		return setVersionOutdated(ctx, ns, arrow.Outdated)
	}
}
