package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type ArrowService struct {
	AddErr                 error
	UpdateResult           models.UpdateResult
	UpdateErr              error
	RemoveErr              error
	ListResult             []models.ArrowListDTO
	ListErr                error
	ListUserInstalledArg   *bool
	GetResult              *domain.Arrow
	GetErr                 error
	GetDetailResult        *models.ArrowDetailDTO
	GetDetailErr           error
	GetManifestResult      *models.ArrowManifestDTO
	GetManifestErr         error
	HasDependentsResult    bool
	HasDependentsErr       error
	SeedErr                error
	ValidateManifestResult *models.ValidationResult
	ValidateManifestErr    error
}

func (m *ArrowService) Add(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.AddErr
}

func (m *ArrowService) Update(
	_ context.Context,
	_ domain.Namespace,
	_ models.UpdateOptions,
) (models.UpdateResult, error) {
	return m.UpdateResult, m.UpdateErr
}

func (m *ArrowService) Remove(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.RemoveErr
}

func (m *ArrowService) List(
	_ context.Context,
	userInstalled *bool,
) ([]models.ArrowListDTO, error) {
	m.ListUserInstalledArg = userInstalled
	return m.ListResult, m.ListErr
}

func (m *ArrowService) Get(
	_ context.Context,
	_ domain.Namespace,
) (*domain.Arrow, error) {
	return m.GetResult, m.GetErr
}

func (m *ArrowService) GetDetail(
	_ context.Context,
	_ domain.Namespace,
) (*models.ArrowDetailDTO, error) {
	return m.GetDetailResult, m.GetDetailErr
}

func (m *ArrowService) GetManifest(
	_ context.Context,
	_ domain.Namespace,
) (*models.ArrowManifestDTO, error) {
	return m.GetManifestResult, m.GetManifestErr
}

func (m *ArrowService) HasDependents(
	_ context.Context,
	_ domain.Namespace,
	_ domain.Namespace,
) (bool, error) {
	return m.HasDependentsResult, m.HasDependentsErr
}

func (m *ArrowService) Seed(
	_ context.Context,
	_ domain.Namespace,
	_ []byte,
) error {
	return m.SeedErr
}

func (m *ArrowService) ValidateManifest(
	_ context.Context,
	_ []byte,
) (*models.ValidationResult, error) {
	return m.ValidateManifestResult, m.ValidateManifestErr
}
