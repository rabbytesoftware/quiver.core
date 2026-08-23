package runtime

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type GetArrowFn func(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error)

type MarkInstalledFn func(
	ctx context.Context,
	ns domain.Namespace,
	at time.Time,
) error

type MarkUninstalledFn func(
	ctx context.Context,
	ns domain.Namespace,
) error

type HasDependentsFn func(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error)

type ListArrowsFn func(ctx context.Context) ([]models.ArrowView, error)

// ListRuntimeAggregatesFn returns the namespaces of every runtime aggregate
// that currently has events in the runtime event store.
type ListRuntimeAggregatesFn func(ctx context.Context) ([]domain.Namespace, error)
