package usecases

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/surface"
)

// ErrNoSurface means the arrow has no interface open right now.
var ErrNoSurface = errors.New("arrow has no open surface")

// SurfaceUsecase finds the handler serving an arrow's open surface.
type SurfaceUsecase interface {
	Handler(ctx context.Context, ns domain.Namespace) (http.Handler, error)
}

type surfaceUsecase struct {
	runtime RuntimeUsecase
	engine  surface.Surface
}

func NewSurfaceUsecase(rt RuntimeUsecase, eng surface.Surface) SurfaceUsecase {
	return &surfaceUsecase{runtime: rt, engine: eng}
}

func (u *surfaceUsecase) Handler(ctx context.Context, ns domain.Namespace) (http.Handler, error) {
	rt, err := u.runtime.GetRuntime(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("surface handler %s: %w", ns, err)
	}
	if rt == nil || rt.Execution == nil || rt.Execution.Surface == nil {
		return nil, fmt.Errorf("surface handler %s: %w", ns, ErrNoSurface)
	}
	s := rt.Execution.Surface
	h, err := u.engine.Handler(surface.Spec{Mode: s.Mode, Namespace: ns, Dir: s.Dir})
	if err != nil {
		return nil, fmt.Errorf("surface handler %s: %w", ns, err)
	}
	return h, nil
}
