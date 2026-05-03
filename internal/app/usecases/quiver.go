package usecases

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/app/models"
	quiverrepo "github.com/rabbytesoftware/quiver/internal/app/repositories/quiver"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

// QuiverUsecase is the public contract for quiver read/write operations.
type QuiverUsecase interface {
	Add(
		ctx context.Context,
		ns domain.Namespace,
	) error

	Update(
		ctx context.Context,
		ns domain.Namespace,
	) error

	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error

	List(
		ctx context.Context,
	) ([]models.QuiverListDTO, error)

	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*models.QuiverDetailDTO, error)
}

type quiverUsecase struct {
	repo quiverrepo.Quiver
}

// NewQuiverUsecase wires the quiver repository into a QuiverUsecase.
func NewQuiverUsecase(repo quiverrepo.Quiver) QuiverUsecase {
	return &quiverUsecase{
		repo: repo,
	}
}

func (u *quiverUsecase) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return u.repo.Add(ctx, ns)
}

func (u *quiverUsecase) Update(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return u.repo.Update(ctx, ns)
}

func (u *quiverUsecase) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return u.repo.Remove(ctx, ns)
}

func (u *quiverUsecase) List(ctx context.Context) ([]models.QuiverListDTO, error) {
	quivers, err := u.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]models.QuiverListDTO, 0, len(quivers))
	for _, q := range quivers {
		result = append(result, models.QuiverListDTO{
			Namespace: q.Namespace,
		})
	}

	return result, nil
}

func (u *quiverUsecase) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*models.QuiverDetailDTO, error) {
	q, err := u.repo.Get(ctx, ns)
	if err != nil {
		return nil, err
	}

	return &models.QuiverDetailDTO{
		Namespace: q.Namespace,
	}, nil
}
