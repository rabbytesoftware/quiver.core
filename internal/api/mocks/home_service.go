package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

type HomeService struct {
	HomeResult   models.Home
	HomeErr      error
	HomeCalls    int
	RefreshCalls int
}

func (m *HomeService) Home(
	_ context.Context,
) (models.Home, error) {
	m.HomeCalls++
	return m.HomeResult, m.HomeErr
}

func (m *HomeService) Refresh(
	_ context.Context,
) {
	m.RefreshCalls++
}
