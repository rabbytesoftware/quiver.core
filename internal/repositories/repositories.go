package repositories

import (
	"github.com/rabbytesoftware/quiver/internal/infrastructure"
	"github.com/rabbytesoftware/quiver/internal/repositories/arrows"
	"github.com/rabbytesoftware/quiver/internal/repositories/quivers"
	"github.com/rabbytesoftware/quiver/internal/repositories/system"
)

type Repositories struct {
	arrows  arrows.ArrowsInterface
	system  system.SystemInterface
	quivers quivers.QuiversInterface
}

func NewRepositories(
	infrastructure *infrastructure.Infrastructure,
) *Repositories {
	return &Repositories{
		arrows:  arrows.NewArrowsRepository(),
		system:  system.NewSystemRepository(infrastructure),
		quivers: quivers.NewQuiversRepository(infrastructure),
	}
}

func (r *Repositories) GetArrows() arrows.ArrowsInterface {
	return r.arrows
}

func (r *Repositories) GetSystem() system.SystemInterface {
	return r.system
}

func (r *Repositories) GetQuivers() quivers.QuiversInterface {
	return r.quivers
}
