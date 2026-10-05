package runtime_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	runtimeMocks "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime/internal/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/surface"
)

type fakeSurface struct {
	readySpec surface.Spec
	readyPath string
	cleaned   []domain.Namespace
}

func (f *fakeSurface) SocketPath(domain.Namespace) string         { return "" }
func (f *fakeSurface) Prepare(domain.Namespace) (string, error)   { return "", nil }
func (f *fakeSurface) Handler(surface.Spec) (http.Handler, error) { return nil, surface.ErrNoSurface }

func (f *fakeSurface) Cleanup(ns domain.Namespace) { f.cleaned = append(f.cleaned, ns) }

func (f *fakeSurface) Ready(
	_ context.Context,
	spec surface.Spec,
	path string,
) bool {
	f.readySpec = spec
	f.readyPath = path
	return true
}

func TestSurfaceHooks_DelegateToTheSurfaceEngine(t *testing.T) {
	surfaces := &fakeSurface{}
	ready, closeSurface := runtime.SurfaceHooks(surfaces)
	ns := domain.Namespace("github.com/user/chat@v1.0.0")

	require.True(t, ready(t.Context(), ns, domainRuntime.Surface{
		Mode: domainRuntime.SurfaceModeStatic,
		Path: "/index.html",
		Dir:  "/srv/dist",
	}))
	closeSurface(ns)

	assert.Equal(t, surface.Spec{Mode: domainRuntime.SurfaceModeStatic, Namespace: ns, Dir: "/srv/dist"}, surfaces.readySpec)
	assert.Equal(t, "/index.html", surfaces.readyPath)
	assert.Equal(t, []domain.Namespace{ns}, surfaces.cleaned)
}

func TestSurfaceHooks_NoEngineLeavesThemUnset(t *testing.T) {
	ready, closeSurface := runtime.SurfaceHooks(nil)

	assert.Nil(t, ready)
	assert.Nil(t, closeSurface)
}

func TestOnRuntimeSurfaceSet_CallbackFires(t *testing.T) {
	axRuntime := newTestAsynxRuntime(t)
	ns := testNs()
	f := catToFuncs(&runtimeMocks.MockArrow{})
	lc, err := runtime.NewTestable(axRuntime, nil, successAssembler(), f.markInstalled, f.markUninstalled, f.markLastUsed, f.hasDependents, f.listArrows, func(context.Context) ([]domain.Namespace, error) { return nil, nil })
	require.NoError(t, err)

	called := make(chan struct{}, 1)
	require.NoError(t, lc.OnRuntimeSurfaceSet(func(_ context.Context, _ domainRuntime.ArrowRuntime) {
		called <- struct{}{}
	}))

	fireAndWait(t, axRuntime, ns, "runtime.surface_set."+ns.String())

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("OnRuntimeSurfaceSet callback not called")
	}
}
