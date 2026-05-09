package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type CollectionService struct {
	ListResult []models.CollectionListDTO
	ListErr    error
	GetResult  *models.CollectionDetailDTO
	GetErr     error

	FollowErr         error
	UnfollowErr       error
	SeedErr           error
	GetManifestResult []byte
	GetManifestErr    error
	ValidateResult    *models.ValidationResult
	ValidateErr       error
}

func (m *CollectionService) Follow(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.FollowErr
}

func (m *CollectionService) Unfollow(
	_ context.Context,
	_ domain.Namespace,
) error {
	return m.UnfollowErr
}

func (m *CollectionService) List(
	_ context.Context,
	_ *bool,
) ([]models.CollectionListDTO, error) {
	return m.ListResult, m.ListErr
}

func (m *CollectionService) Get(
	_ context.Context,
	_ domain.Namespace,
) (*models.CollectionDetailDTO, error) {
	return m.GetResult, m.GetErr
}

func (m *CollectionService) Seed(
	_ context.Context,
	_ domain.Namespace,
	_ []byte,
) error {
	return m.SeedErr
}

func (m *CollectionService) GetManifest(
	_ context.Context,
	_ domain.Namespace,
) ([]byte, error) {
	return m.GetManifestResult, m.GetManifestErr
}

func (m *CollectionService) ValidateManifest(
	_ context.Context,
	_ []byte,
) (*models.ValidationResult, error) {
	return m.ValidateResult, m.ValidateErr
}
