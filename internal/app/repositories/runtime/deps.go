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
	ref string,
	at time.Time,
) error

type HasDependentsFn func(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error)

type ListArrowsFn func(ctx context.Context) ([]models.ArrowView, error)
