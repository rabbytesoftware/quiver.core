// Package ui implements the ui step: it validates the surface the step
// declares and reports it to the app layer. It serves nothing itself.
package ui

import (
	"context"
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
	// ErrNoSocket means the method ran a listening ui step without the
	// resolver having provisioned ${ARROW_UI_LISTEN}.
	ErrNoSocket = errors.New("ui: ARROW_UI_LISTEN was not provisioned")
	// ErrBadStaticDir means the static directory is missing, not a
	// directory, or outside the workdir.
	ErrBadStaticDir = errors.New("ui: static directory is not usable")
)

type handler struct{}

func NewHandler() wizstep.Handler[domainstep.UIStep] { return handler{} }

func (handler) Execute(_ context.Context, req wizstep.Request, s domainstep.UIStep) error {
	surface := domainRuntime.Surface{Path: s.Path}
	if surface.Path == "" {
		surface.Path = "/"
	}

	switch {
	case len(s.Listen) > 0:
		if req.Vars[domain.VarArrowUIListen] == "" {
			return ErrNoSocket
		}
		surface.Mode = domainRuntime.SurfaceModeListen
	default:
		dir, err := resolveStatic(req.WorkDir, req.Expand(s.Static))
		if err != nil {
			return err
		}
		surface.Mode = domainRuntime.SurfaceModeStatic
		surface.Dir = dir
		surface.Ready = true
	}

	req.Emit(models.Event{Kind: models.EventKindSurface, Surface: &surface})
	return nil
}

func resolveStatic(workdir, rel string) (string, error) {
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
