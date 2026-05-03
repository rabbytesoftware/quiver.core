package usecases

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/app/repositories"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type Container struct {
	Arrow   ArrowUsecase
	Runtime RuntimeUsecase
	Quiver  QuiverUsecase
}

func New(repos *repositories.Container) (*Container, error) {
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
	quiverUC := NewQuiverUsecase(
		repos.Quiver,
		repos.Arrow,
		repos.Manifold,
		repos.Vault,
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
		Arrow:   arrowUC,
		Runtime: runtimeUC,
		Quiver:  quiverUC,
	}, nil
}
