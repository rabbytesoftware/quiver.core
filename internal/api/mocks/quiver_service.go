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

func (m *QuiverService) List(_ context.Context) ([]models.QuiverListDTO, error) {
	return m.ListResult, m.ListErr
}

func (m *QuiverService) Get(
	_ context.Context,
	_ domain.Namespace,
) (*models.QuiverDetailDTO, error) {
	return m.GetResult, m.GetErr
}
