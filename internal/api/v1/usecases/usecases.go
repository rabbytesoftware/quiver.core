package usecases

import (
	"github.com/rabbytesoftware/quiver/internal/api/v1/usecases/arrows"
	"github.com/rabbytesoftware/quiver/internal/api/v1/usecases/quivers"
	"github.com/rabbytesoftware/quiver/internal/api/v1/usecases/system"
	"github.com/rabbytesoftware/quiver/internal/repositories"
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
