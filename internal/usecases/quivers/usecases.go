package quivers

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/repositories"
	"github.com/rabbytesoftware/quiver/internal/repositories/quivers"
)

type ApiQuiverUsescases struct {
	rp  quivers.QuiversInterface
	ctx context.Context
}

func NewApiQuiversUsecases(repos *repositories.Repositories) *ApiQuiverUsescases {

	return &ApiQuiverUsescases{
		rp:  repos.GetQuivers(),
		ctx: context.Background(),
	}
}
