package extract

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
)

var ErrPortableFormat = errors.New("portable app format, use type: portable")

type handler struct {
	maxBytes int64
}

func NewHandler(
	maxBytes int64,
) wizstep.Handler[domainstep.ExtractStep] {
	return &handler{maxBytes: maxBytes}
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.ExtractStep,
) error {
	osArch := req.OSArch.String()

	stepCtx, cancel, err := withTimeout(ctx, s.Timeout.Resolve(osArch))
	if err != nil {
		return err
	}
	defer cancel()

	from := resolvePath(req, s.From.Resolve(osArch))
	to := resolvePath(req, s.To.Resolve(osArch))

	src, err := os.Open(from) //nolint:gosec
	if err != nil {
		return fmt.Errorf("extract: open %s: %w", from, err)
	}
	defer src.Close() //nolint:errcheck

	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("extract: stat %s: %w", from, err)
	}

	if unpack.IsAppImage(src) || unpack.IsDmg(src, info.Size()) {
		return fmt.Errorf("extract: %s: %w", from, ErrPortableFormat)
	}

	archive, err := unpack.DetectArchive(src, info.Size())
	if err != nil {
		return err
	}

	g, err := unpack.OpenGuard(ctx, to, h.maxBytes)
	if err != nil {
		return err
	}
	defer g.Close()

	return errors.Join(archive.Extract(stepCtx, src, info.Size(), g), g.Verify())
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
		return nil, nil, fmt.Errorf("extract: invalid timeout %q: %w", raw, err)
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
