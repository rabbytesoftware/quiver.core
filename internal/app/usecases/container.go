package usecases

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

type Container struct {
	Arrow      ArrowUsecase
	Runtime    RuntimeUsecase
	Collection CollectionUsecase
}

func New(repos *repositories.Container, m manifold.Manifold, v vault.Vault) (*Container, error) {
	arrowUC := NewArrowUsecase(
		repos.Arrow,
		repos.Graph,
		repos.Runtime,
	)
	runtimeUC := &runtimeUsecase{
		arrow:   repos.Arrow,
		runtime: repos.Runtime,
		graph:   repos.Graph,
	}
	quiverUC := NewCollectionUsecase(
		repos.Collection,
		repos.Arrow,
		m,
		v,
	)

	if err := repos.Runtime.OnRuntimeEnded(func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	) {
		runtimeUC.onRuntimeEnded(ctx, rt)
	}); err != nil {
		return nil, fmt.Errorf("usecases: wire OnRuntimeEnded: %w", err)
	}

	if err := repos.Arrow.OnArrowUpgraded(func(
		ctx context.Context,
		arrow domain.Arrow,
	) error {
		runtimeUC.onArrowUpgraded(ctx, arrow)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("usecases: wire OnArrowUpgraded: %w", err)
	}

	return &Container{
		Arrow:      arrowUC,
		Runtime:    runtimeUC,
		Collection: quiverUC,
	}, nil
}
