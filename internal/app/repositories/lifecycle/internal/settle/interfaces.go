package settle

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type Arrow interface {
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	TargetUnmoved(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) (bool, error)
	Advance(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) error
	RefreshToTarget(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) (*domain.Arrow, error)
}

type Runtime interface {
	ReconcileVersionBadge(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

// Targets is what settling needs from the targets update brackets remembered.
type Targets interface {
	Pending(ns domain.Namespace) bool
	Take(ns domain.Namespace) (domain.Available, bool)
}

// Commits is what settling needs from the tracking of commits in flight.
type Commits interface {
	Begin(ns domain.Namespace) bool
	Done(ns domain.Namespace)
	InFlight(ns domain.Namespace) bool
	IsDraining() bool
	Bound(
		parent context.Context,
		timeout time.Duration,
	) (context.Context, context.CancelFunc)
}

// Stager records what an update left waiting for a daemon restart.
type Stager interface {
	Stage(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
		target domain.Available,
	) error
}
