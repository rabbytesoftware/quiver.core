package portable

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
)

type handler struct {
	maxBytes int64
}

func NewHandler(
	maxBytes int64,
) wizstep.Handler[domainstep.PortableStep] {
	return &handler{maxBytes: maxBytes}
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.PortableStep,
) error {
	osArch := req.OSArch.String()

	stepCtx, cancel, err := withTimeout(ctx, s.Timeout.Resolve(osArch))
	if err != nil {
		return err
	}
	defer cancel()

	from := resolvePath(req, s.From.Resolve(osArch))
	to := resolvePath(req, s.To.Resolve(osArch))

	apps, output, err := h.install(stepCtx, req.OSArch, from, to)
	if err != nil {
		return err
	}

	if err := recordApps(stepCtx, req.NSKey, req.WorkDir, apps); err != nil {
		return err
	}

	return removeSource(req.WorkDir, from, output)
}

func (h *handler) install(
	ctx context.Context,
	osArch domain.OS,
	from string,
	to string,
) ([]domain.PortableApp, string, error) {
	src, err := os.Open(from) //nolint:gosec
	if err != nil {
		return nil, "", fmt.Errorf("portable: open %s: %w", from, err)
	}
	defer src.Close() //nolint:errcheck

	info, err := src.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("portable: stat %s: %w", from, err)
	}

	kind, archive, err := detectFormat(src, info.Size())
	if err != nil {
		return nil, "", err
	}

	var apps []domain.PortableApp
	switch kind {
	case formatAppImage:
		apps, err = h.installAppImage(ctx, src, info.Size(), from, to)
		return apps, "", err
	case formatDmg:
		apps, err = h.installDmg(ctx, from, to)
		return apps, "", err
	case formatArchive:
		apps, err = h.installArchive(ctx, osArch, src, info.Size(), archive, to)
		return apps, "", err
	case formatExecutable:
		output, err := h.installExecutable(ctx, src, info, from, to)
		return nil, output, err
	case formatUnknown:
	}

	return nil, "", fmt.Errorf("%w: %s", ErrUnknownFormat, from)
}

func withTimeout(
	ctx context.Context,
	raw string,
) (context.Context, context.CancelFunc, error) {
	if raw == "" {
		stepCtx, cancel := context.WithCancel(ctx)
		return stepCtx, cancel, nil
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("portable: invalid timeout %q: %w", raw, err)
	}

	stepCtx, cancel := context.WithTimeout(ctx, d)

	return stepCtx, cancel, nil
}

func resolvePath(
	req wizstep.Request,
	raw string,
) string {
	path := req.Expand(raw)
	if filepath.IsAbs(path) {
		return path
	}

	return filepath.Join(req.WorkDir, path)
}
