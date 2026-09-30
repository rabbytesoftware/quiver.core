package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

type Container struct {
	Arrow      ArrowUsecase
	Runtime    RuntimeUsecase
	Collection CollectionUsecase
	Search     SearchUsecase
	// Discovery is nil when the container was built without a vault or a
	// manifold, mirroring the repository that has nothing to run.
	Discovery DiscoveryUsecase
	Config    ConfigUsecase
	Auth      AuthUsecase
	Path      PathUsecase
}

func New(
	repos *repositories.Container,
	m manifold.Manifold,
	v vault.Vault,
	w wizardPkg.Wizard,
) (*Container, error) {
	pairingCodeTTL, err := time.ParseDuration(config.GetAuth().PairingCodeTTL)
	if err != nil {
		return nil, fmt.Errorf("usecases: parse auth.pairing_code_ttl: %w", err)
	}
	runtimeUC := newRuntimeUsecase(
		repos.Arrow,
		repos.Runtime,
		repos.Graph,
	)
	arrowUC := newArrowUsecase(
		repos.Arrow,
		repos.Graph,
		repos.Runtime,
		runtimeUC.targets,
	)
	quiverUC := NewCollectionUsecase(
		repos.Collection,
		repos.Arrow,
		m,
		v,
	)
	searchUC := NewSearchUsecase(
		repos.Arrow,
		v,
		repos.Collection,
	)

	var discoveryUC DiscoveryUsecase
	if repos.Discovery != nil {
		discoveryUC = NewDiscoveryUsecase(repos.Discovery)
	}

	if err := repos.Runtime.OnRuntimeEnded(func(
		ctx context.Context,
		rt domainRuntime.ArrowRuntime,
	) {
		runtimeUC.onRuntimeEnded(ctx, rt)
	}); err != nil {
		return nil, fmt.Errorf("usecases: wire OnRuntimeEnded: %w", err)
	}

	return &Container{
		Arrow:      arrowUC,
		Runtime:    runtimeUC,
		Collection: quiverUC,
		Search:     searchUC,
		Discovery:  discoveryUC,
		Config:     NewConfigUsecase(repos.Config),
		Auth:       NewAuthUsecase(repos.PairingCode, repos.Device, pairingCodeTTL),
		Path:       NewPathUsecase(w),
	}, nil
}
