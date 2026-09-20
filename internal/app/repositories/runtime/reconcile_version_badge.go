package runtime

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// ReconcileVersionBadge returns the function the drain calls once an execution
// has ended, to put back the outdated badge EndExecution just dropped.
//
// Arrow.Outdated is where the system keeps its answer to "does this arrow have
// a newer release available", written by the passive version check that
// GetDetail triggers. ArrowRuntime.State is a projection of that answer, kept
// in step by SetVersionOutdated, and it is the one the list and WebSocket views
// read. EndExecution rewrites that state from the method and outcome alone, so
// an arrow that was Outdated when it started comes back Ready — and stays
// unbadged there until the next version check is due, which is up to a TTL
// away. This reads the answer that never changed and re-derives the projection
// from it.
//
// It takes the answer from the catalog rather than re-resolving the remote on
// purpose; see reconcileVersionBadge in runtime/internal/hooks.go for why a
// fresh check here would buy nothing for the round trip it costs.
//
// It reuses SetVersionOutdated, so which transitions are legal stays the
// aggregate's own business: a namespace with no runtime, an arrow busy doing
// something else, and an Outdated a dependency sync put there are all left
// exactly as they are. An arrow the catalog no longer holds — forgotten while
// it ran — has no answer to project, and the error says so rather than being
// taken for "not outdated".
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
