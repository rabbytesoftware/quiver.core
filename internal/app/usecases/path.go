package usecases

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf"
)

type PathUsecase interface {
	Status(
		ctx context.Context,
	) (models.PathStatus, error)
	Setup(
		ctx context.Context,
	) (models.PathStatus, error)
}

type pathUsecase struct {
	shelf shelf.Shelf
}

func NewPathUsecase(
	s shelf.Shelf,
) PathUsecase {
	return &pathUsecase{shelf: s}
}

func (u *pathUsecase) Status(
	ctx context.Context,
) (models.PathStatus, error) {
	status, err := u.shelf.PathStatus(ctx)
	if err != nil {
		return models.PathStatus{}, fmt.Errorf("path status: %w", err)
	}
	return pathStatusFrom(status), nil
}

func (u *pathUsecase) Setup(
	ctx context.Context,
) (models.PathStatus, error) {
	status, err := u.shelf.SetupPath(ctx)
	if err != nil {
		return models.PathStatus{}, fmt.Errorf("path setup: %w", err)
	}
	return pathStatusFrom(status), nil
}

func pathStatusFrom(
	status shelf.PathStatus,
) models.PathStatus {
	return models.PathStatus{
		BinDir:     status.BinDir,
		OnPath:     status.OnPath,
		Configured: status.Configured,
		Files:      status.Files,
	}
}
