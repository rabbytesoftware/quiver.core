package download

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/fns"
	"github.com/rabbytesoftware/quiver/internal/core/fns/config"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step"
)

type handler struct{}

func NewHandler() wizstep.Handler[domainstep.FetchStep] {
	return &handler{}
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.FetchStep,
) error {
	stepCtx := ctx
	var cancel context.CancelFunc

	var downloadOpts []config.Option

	if ts := s.Timeout.Resolve(req.OSArch.String()); ts != "" {
		d, err := time.ParseDuration(ts)
		if err != nil {
			return fmt.Errorf("invalid timeout %q: %w", ts, err)
		}

		stepCtx, cancel = context.WithTimeout(ctx, d)
		defer cancel()

		// Disable the HTTP client's own Timeout so the context deadline is the
		// sole authority. Without this, the default 30s client timeout fires
		// before any step timeout longer than 30s can take effect.
		downloadOpts = append(downloadOpts, config.WithTimeout(0))
	}

	expand := func(key string) string { return req.Vars[key] }

	dst := os.Expand(s.To.Resolve(req.OSArch.String()), expand)
	if !filepath.IsAbs(dst) {
		dst = filepath.Join(req.WorkDir, dst)
	}

	url := os.Expand(s.URL.Resolve(req.OSArch.String()), expand)

	return fns.Download(
		stepCtx,
		url,
		dst,
		nil,
		downloadOpts...,
	)
}
