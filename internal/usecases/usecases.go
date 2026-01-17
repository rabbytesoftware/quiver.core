package usecases

import (
	"github.com/rabbytesoftware/quiver/internal/repositories"
	"github.com/rabbytesoftware/quiver/internal/usecases/arrows"
	"github.com/rabbytesoftware/quiver/internal/usecases/quivers"
	"github.com/rabbytesoftware/quiver/internal/usecases/system"
)

type Usescases struct {
	Arrows  *arrows.ArrowUsecases
	Quivers *quivers.QuiverUsescases
	System  *system.SystemUsescases
}

func NewUsecases(repos *repositories.Repositories) *Usescases {
	return &Usescases{
		Arrows:  arrows.NewArrowsUsecases(repos),
		Quivers: quivers.NewQuiversUsecases(repos),
		System:  system.NewSystemUsecases(repos),
	}
}
