package run

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/models"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
)

var (
	// ErrNoSocket means a run with a listening ui started without the
	// resolver having provisioned ${ARROW_UI_LISTEN}.
	ErrNoSocket = errors.New("run: ARROW_UI_LISTEN was not provisioned")
	// ErrBadStaticDir means the static directory is missing, not a
	// directory, or outside the workdir.
	ErrBadStaticDir = errors.New("run: ui static directory is not usable")
)

// openSurface validates the run's ui and reports it open. It returns the func
// that reports it closed, to be deferred so every way the run can end closes
// the surface. A run without ui opens nothing and returns a no-op.
func openSurface(
	req wizstep.Request,
	ui *domainstep.UIOptions,
) (func(), error) {
	if ui == nil {
		return func() {}, nil
	}
	surface, err := surfaceFor(req, *ui)
	if err != nil {
		return nil, err
	}
	emit(req, models.Event{Kind: models.EventKindSurface, Surface: &surface})
	return func() { emit(req, models.Event{Kind: models.EventKindSurfaceClosed}) }, nil
}

func surfaceFor(
	req wizstep.Request,
	ui domainstep.UIOptions,
) (domainRuntime.Surface, error) {
	surface := domainRuntime.Surface{Title: ui.Title, Path: ui.Path}
	if surface.Path == "" {
		surface.Path = "/"
	}
	if ui.Listens() {
		if req.Vars[domain.VarArrowUIListen] == "" {
			return surface, ErrNoSocket
		}
		surface.Mode = domainRuntime.SurfaceModeListen
		return surface, nil
	}
	dir, err := resolveStatic(req.WorkDir, req.Expand(ui.Static))
	if err != nil {
		return surface, err
	}
	surface.Mode = domainRuntime.SurfaceModeStatic
	surface.Dir = dir
	surface.Ready = true
	return surface, nil
}

func emit(
	req wizstep.Request,
	ev models.Event,
) {
	if req.Emit != nil {
		req.Emit(ev)
	}
}

func resolveStatic(
	workdir string,
	rel string,
) (string, error) {
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%w: %q must stay inside the install directory", ErrBadStaticDir, rel)
	}
	abs := filepath.Join(workdir, rel)

	// Resolve symlinks on both sides so a link out of the workdir is refused.
	realWork, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrBadStaticDir, err)
	}
	realDir, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrBadStaticDir, err)
	}
	if relToWork, err := filepath.Rel(realWork, realDir); err != nil || !filepath.IsLocal(relToWork) && relToWork != "." {
		return "", fmt.Errorf("%w: %q resolves outside the install directory", ErrBadStaticDir, rel)
	}
	if info, err := os.Stat(realDir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: %q is not a directory", ErrBadStaticDir, rel)
	}
	return realDir, nil
}
