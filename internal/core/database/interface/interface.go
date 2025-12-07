package database

import (
	"context"

	"github.com/google/uuid"
)

type RepositoryInterface[T any] interface {
	Get(
		ctx context.Context,
	) ([]*T, error)
	GetByID(
		ctx context.Context,
		id uuid.UUID,
	) (*T, error)
	Create(
		ctx context.Context,
		entity *T,
	) (*T, error)
	Update(
		ctx context.Context,
		entity *T,
	) (*T, error)
	Delete(
		ctx context.Context,
		id uuid.UUID,
	) error
	Exists(
		ctx context.Context,
		id uuid.UUID,
	) (bool, error)
	Count(
		ctx context.Context,
	) (int64, error)
}
