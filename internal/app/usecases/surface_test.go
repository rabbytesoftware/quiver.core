package usecases_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/surface"
)

// stubRuntimeUsecase answers GetRuntime only; the embedded nil interface
// panics if the surface usecase ever reaches for anything else.
type stubRuntimeUsecase struct {
	usecases.RuntimeUsecase
	rt  *domainRuntime.ArrowRuntime
	err error
}

func (s stubRuntimeUsecase) GetRuntime(context.Context, domain.Namespace) (*domainRuntime.ArrowRuntime, error) {
	return s.rt, s.err
}

func runtimeReturning(rt *domainRuntime.ArrowRuntime, err error) usecases.RuntimeUsecase {
	return stubRuntimeUsecase{rt: rt, err: err}
}

func TestSurfaceUsecase_Handler(t *testing.T) {
	ns := domain.Namespace("github.com/user/chat")
	eng := surface.New(t.TempDir())

	t.Run("no runtime record", func(t *testing.T) {
		_, err := usecases.NewSurfaceUsecase(runtimeReturning(nil, nil), eng).Handler(t.Context(), ns)
		require.ErrorIs(t, err, usecases.ErrNoSurface)
	})

	t.Run("no execution", func(t *testing.T) {
		rt := runtimeReturning(&domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady}, nil)
		_, err := usecases.NewSurfaceUsecase(rt, eng).Handler(t.Context(), ns)
		require.ErrorIs(t, err, usecases.ErrNoSurface)
	})

	t.Run("execution without surface", func(t *testing.T) {
		rt := runtimeReturning(&domainRuntime.ArrowRuntime{
			Ref: ns, Execution: &domainRuntime.Execution{ID: "e1"},
		}, nil)
		_, err := usecases.NewSurfaceUsecase(rt, eng).Handler(t.Context(), ns)
		require.ErrorIs(t, err, usecases.ErrNoSurface)
	})

	t.Run("runtime lookup error is passed through", func(t *testing.T) {
		boom := errors.New("boom")
		_, err := usecases.NewSurfaceUsecase(runtimeReturning(nil, boom), eng).Handler(t.Context(), ns)
		require.ErrorIs(t, err, boom)
	})

	t.Run("engine error is passed through", func(t *testing.T) {
		rt := runtimeReturning(&domainRuntime.ArrowRuntime{
			Ref:       ns,
			Execution: &domainRuntime.Execution{ID: "e1", Surface: &domainRuntime.Surface{}},
		}, nil)
		_, err := usecases.NewSurfaceUsecase(rt, eng).Handler(t.Context(), ns)
		require.ErrorIs(t, err, surface.ErrNoSurface)
	})

	t.Run("open surface yields a handler", func(t *testing.T) {
		rt := runtimeReturning(&domainRuntime.ArrowRuntime{
			Ref: ns,
			Execution: &domainRuntime.Execution{ID: "e1", Surface: &domainRuntime.Surface{
				Mode: domainRuntime.SurfaceModeListen, Path: "/",
			}},
		}, nil)
		h, err := usecases.NewSurfaceUsecase(rt, eng).Handler(t.Context(), ns)
		require.NoError(t, err)
		require.NotNil(t, h)
	})
}
