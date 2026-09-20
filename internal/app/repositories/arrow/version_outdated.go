package arrow

import (
	"context"
	"log/slog"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// SetVersionOutdatedFn reconciles ns's runtime state with what a version check
// just found. See runtime.SetVersionOutdated for the implementation the
// container wires in, and why the answer is passed as a plain bool rather than
// as an instruction: which transitions are legal belongs to the runtime
// aggregate, not here.
type SetVersionOutdatedFn func(
	ctx context.Context,
	ns domain.Namespace,
	outdated bool,
) error

// WithVersionOutdatedSync makes a version check's answer reach the aggregate
// the frontend's outdated badge actually reads. Without it — and every caller
// that builds a catalog without a runtime to talk to is without it — a version
// check records its finding on the Arrow aggregate alone, exactly as it always
// has.
func WithVersionOutdatedSync(
	fn SetVersionOutdatedFn,
) Option {
	return func(o *options) {
		o.versionOutdatedSync = fn
	}
}

// syncVersionOutdated pushes the drift answer onto the runtime aggregate. The
// error is logged rather than returned because runVersionCheck has no caller
// to return it to, and because losing this write is recoverable: the sync is
// unconditional, so the next check reconciles whatever this one could not.
func (s *arrowService) syncVersionOutdated(
	ctx context.Context,
	ns domain.Namespace,
	outdated bool,
) {
	if s.versionOutdatedSync == nil {
		return
	}

	if err := s.versionOutdatedSync(ctx, ns, outdated); err != nil {
		slog.WarnContext(ctx, "arrow version check: sync runtime state",
			"ns", ns, "outdated", outdated, "err", err)
	}
}
