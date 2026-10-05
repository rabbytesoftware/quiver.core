package mocks

import (
	"context"
	"net/http"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// SurfaceService is a hand-written test double for usecases.SurfaceUsecase.
type SurfaceService struct {
	HandlerResult http.Handler
	HandlerErr    error
}

func (m *SurfaceService) Handler(
	context.Context,
	domain.Namespace,
) (http.Handler, error) {
	return m.HandlerResult, m.HandlerErr
}
