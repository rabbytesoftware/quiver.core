package system

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/repositories"
	"github.com/rabbytesoftware/quiver/internal/repositories/system"
)

type SystemUsescases struct {
	rp  system.SystemInterface
	ctx context.Context
}

func NewSystemUsecases(repos *repositories.Repositories) *SystemUsescases {

	return &SystemUsescases{
		rp:  repos.GetSystem(),
		ctx: context.Background(),
	}
}
