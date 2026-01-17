package quivers

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/repositories"
	"github.com/rabbytesoftware/quiver/internal/repositories/quivers"
)

type QuiverUsescases struct {
	rp  quivers.QuiversInterface
	ctx context.Context
}

func NewQuiversUsecases(repos *repositories.Repositories) *QuiverUsescases {

	return &QuiverUsescases{
		rp:  repos.GetQuivers(),
		ctx: context.Background(),
	}
}
