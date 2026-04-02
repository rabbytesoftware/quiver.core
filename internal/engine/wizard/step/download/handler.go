package download

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/fns"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
)

type handler struct{}

func NewHandler() wizstep.Handler {
	return &handler{}
}

func (h *handler) ShouldExecute(
	t domainstep.StepType,
) bool {
	return t == domainstep.StepTypeFetch
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.Step,
) error {
	fs := s.(domainstep.FetchStep)

	stepCtx := ctx
	var cancel context.CancelFunc
	if ts := fs.Timeout.Resolve(req.OSArch.String()); ts != "" {
		d, err := time.ParseDuration(ts)
		if err != nil {
			return fmt.Errorf("invalid timeout %q: %w", ts, err)
		}
		stepCtx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}

	dst := fs.To.Resolve(req.OSArch.String())
	if !filepath.IsAbs(dst) {
		dst = filepath.Join(req.WorkDir, dst)
	}

	return fns.Download(stepCtx, fs.URL.Resolve(req.OSArch.String()), dst, nil)
}
