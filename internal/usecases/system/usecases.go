package system

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/repositories"
	"github.com/rabbytesoftware/quiver/internal/repositories/system"
)

type ApiSystemUsescases struct {
	rp  system.SystemInterface
	ctx context.Context
}

func NewApiSystemUsecases(repos *repositories.Repositories) *ApiSystemUsescases {

	return &ApiSystemUsescases{
		rp:  repos.GetSystem(),
		ctx: context.Background(),
	}
}
