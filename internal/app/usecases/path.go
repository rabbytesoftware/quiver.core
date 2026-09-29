package usecases

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
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
	wizard wizardPkg.Wizard
}

func NewPathUsecase(
	w wizardPkg.Wizard,
) PathUsecase {
	return &pathUsecase{wizard: w}
}

func (u *pathUsecase) Status(
	ctx context.Context,
) (models.PathStatus, error) {
	status, err := u.wizard.PathStatus(ctx)
	if err != nil {
		return models.PathStatus{}, fmt.Errorf("path status: %w", err)
	}
	return pathStatusFrom(status), nil
}

func (u *pathUsecase) Setup(
	ctx context.Context,
) (models.PathStatus, error) {
	status, err := u.wizard.SetupPath(ctx)
	if err != nil {
		return models.PathStatus{}, fmt.Errorf("path setup: %w", err)
	}
	return pathStatusFrom(status), nil
}

func pathStatusFrom(
	status wizardPkg.PathStatus,
) models.PathStatus {
	return models.PathStatus{
		BinDir:     status.BinDir,
		OnPath:     status.OnPath,
		Configured: status.Configured,
		Files:      status.Files,
	}
}
