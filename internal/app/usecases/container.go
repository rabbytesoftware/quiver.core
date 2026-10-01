package usecases

import (
	"fmt"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
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
	// Home is nil when the container was built without discovery, since there
	// is nothing to recommend from.
	Home HomeUsecase
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
	runtimeUC := NewRuntimeUsecase(
		repos.Arrow,
		repos.Runtime,
		repos.Lifecycle,
	)
	arrowUC := NewArrowUsecase(
		repos.Arrow,
		repos.Graph,
		repos.Runtime,
		repos.Lifecycle,
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

	var homeUC HomeUsecase
	if repos.Recommendation != nil {
		homeUC = NewHomeUsecase(repos.Recommendation)
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
		Home:       homeUC,
	}, nil
}
