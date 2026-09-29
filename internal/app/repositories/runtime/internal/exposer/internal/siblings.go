package exposerinternal

import (
	"context"
	"errors"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type GetArrowFn func(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error)

type ListArrowsFn func(ctx context.Context) ([]models.ArrowView, error)

func Installed(
	ctx context.Context,
	ns domain.Namespace,
	getArrow GetArrowFn,
	listArrows ListArrowsFn,
) (bool, error) {
	views, err := listArrows(ctx)
	if err != nil {
		return false, err
	}
	for _, view := range views {
		if viewHasInstalled(ctx, view, ns, getArrow) {
			return true, nil
		}
	}
	return false, nil
}

func viewHasInstalled(
	ctx context.Context,
	view models.ArrowView,
	ns domain.Namespace,
	getArrow GetArrowFn,
) bool {
	for _, ver := range view.Versions {
		if ver.Namespace == ns || ver.Namespace.BareNamespace() != ns.BareNamespace() {
			continue
		}
		if installed(ctx, ver.Namespace, getArrow) {
			return true
		}
	}
	return false
}

func installed(
	ctx context.Context,
	ns domain.Namespace,
	getArrow GetArrowFn,
) bool {
	arrow, err := getArrow(ctx, ns)
	if errors.Is(err, asynxModels.ErrNotFound) {
		return false
	}
	if err != nil {
		return true
	}
	return arrow != nil && !arrow.InstalledAt.IsZero()
}
