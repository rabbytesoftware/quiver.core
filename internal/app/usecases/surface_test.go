package usecases_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
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

func (s stubRuntimeUsecase) GetRuntime(
	context.Context,
	domain.Namespace,
) (*domainRuntime.ArrowRuntime, error) {
	return s.rt, s.err
}

func runtimeReturning(
	rt *domainRuntime.ArrowRuntime,
	err error,
) usecases.RuntimeUsecase {
	return stubRuntimeUsecase{rt: rt, err: err}
}

func TestSurfaceUsecase_Handler(t *testing.T) {
	ns := domain.Namespace("github.com/user/chat")
	eng := surface.New(t.TempDir())

	t.Run("no runtime record", func(t *testing.T) {
		_, err := usecases.NewSurfaceUsecase(runtimeReturning(nil, nil), eng).Handler(t.Context(), ns)
		require.ErrorIs(t, err, apperrors.ErrNoSurface)
	})

	t.Run("no execution", func(t *testing.T) {
		rt := runtimeReturning(&domainRuntime.ArrowRuntime{Ref: ns, State: domain.ArrowStateReady}, nil)
		_, err := usecases.NewSurfaceUsecase(rt, eng).Handler(t.Context(), ns)
		require.ErrorIs(t, err, apperrors.ErrNoSurface)
	})

	t.Run("execution without surface", func(t *testing.T) {
		rt := runtimeReturning(&domainRuntime.ArrowRuntime{
			Ref: ns, Execution: &domainRuntime.Execution{ID: "e1"},
		}, nil)
		_, err := usecases.NewSurfaceUsecase(rt, eng).Handler(t.Context(), ns)
		require.ErrorIs(t, err, apperrors.ErrNoSurface)
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

func TestSurfaceUsecase_Handler_TargetsSocketOfResolvedRef(t *testing.T) {
	bare := domain.Namespace("github.com/user/chat")
	resolved := domain.Namespace("github.com/user/chat@stable")
	dir, err := os.MkdirTemp("", "qsu")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	eng := surface.New(dir)

	socket, err := eng.Prepare(resolved)
	require.NoError(t, err)
	ln, err := net.Listen("unix", socket)
	require.NoError(t, err)
	srv := &httptest.Server{Listener: ln, Config: &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("resolved"))
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}}
	srv.Start()
	t.Cleanup(srv.Close)

	rt := runtimeReturning(&domainRuntime.ArrowRuntime{
		Ref: resolved,
		Execution: &domainRuntime.Execution{ID: "e1", Surface: &domainRuntime.Surface{
			Mode: domainRuntime.SurfaceModeListen, Path: "/",
		}},
	}, nil)
	h, err := usecases.NewSurfaceUsecase(rt, eng).Handler(t.Context(), bare)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "resolved", rec.Body.String())
}
