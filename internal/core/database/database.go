package database

import (
	"context"

	interfaces "github.com/rabbytesoftware/quiver/internal/core/database/interface"
	"github.com/rabbytesoftware/quiver/internal/core/database/repository"
)

func NewDatabase[T any](
	ctx context.Context,
	name string,
) (interfaces.RepositoryInterface[T], error) {
	return repository.NewRepository[T](name)
}
