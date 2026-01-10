package usecases

import (
	"github.com/rabbytesoftware/quiver/internal/repositories"
	"github.com/rabbytesoftware/quiver/internal/usecases/arrows"
	"github.com/rabbytesoftware/quiver/internal/usecases/quivers"
	"github.com/rabbytesoftware/quiver/internal/usecases/system"
)

type ApiUsescases struct {
	Arrows  *arrows.ApiArrowUsecases
	Quivers *quivers.ApiQuiverUsescases
	System  *system.ApiSystemUsescases
}

func NewApiUsecases(repos *repositories.Repositories) *ApiUsescases {
	return &ApiUsescases{
		Arrows:  arrows.NewApiArrowsUsecases(repos),
		Quivers: quivers.NewApiQuiversUsecases(repos),
		System:  system.NewApiSystemUsecases(repos),
	}
}
