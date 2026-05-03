package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type QuiverService struct {
	AddErr     error
	UpdateErr  error
	RemoveErr  error
	ListResult []models.QuiverListDTO
	ListErr    error
	GetResult  *models.QuiverDetailDTO
	GetErr     error

	FollowErr         error
	UnfollowErr       error
	SeedErr           error
	GetManifestResult []byte
	GetManifestErr    error
	ValidateResult    *models.ValidationResult
	ValidateErr       error
}

func (m *QuiverService) Add(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.AddErr
}

func (m *QuiverService) Update(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.UpdateErr
}

func (m *QuiverService) Remove(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.RemoveErr
}

func (m *QuiverService) Follow(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.FollowErr
}

func (m *QuiverService) Unfollow(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.UnfollowErr
}

func (m *QuiverService) List(
	_ context.Context,
	_ *bool,
) ([]models.QuiverListDTO, error) {
	return m.ListResult, m.ListErr
}

func (m *QuiverService) Get(
	_ context.Context,
	_ domain.Namespace,
) (*models.QuiverDetailDTO, error) {
	return m.GetResult, m.GetErr
}

func (m *QuiverService) Seed(
	_ context.Context,
	_ domain.Namespace,
	_ []byte,
) error {
	return m.SeedErr
}

func (m *QuiverService) GetManifest(
	_ context.Context,
	_ domain.Namespace,
) ([]byte, error) {
	return m.GetManifestResult, m.GetManifestErr
}

func (m *QuiverService) ValidateManifest(
	_ context.Context,
	_ []byte,
) (*models.ValidationResult, error) {
	return m.ValidateResult, m.ValidateErr
}
