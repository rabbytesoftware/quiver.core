package download

import (
	"context"
	"path/filepath"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/core/fns"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) ShouldExecute(t domainstep.StepType) bool {
	return t == domainstep.StepTypeFetch
}

func (h *Handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.Step,
) error {
	fs := s.(domainstep.FetchStep)

	stepCtx := ctx
	var cancel context.CancelFunc
	if fs.Timeout > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, fs.Timeout)
		defer cancel()
	}

	dst := fs.To
	if !filepath.IsAbs(dst) {
		dst = filepath.Join(req.WorkDir, dst)
	}

	return fns.Download(stepCtx, fs.URL, dst, nil)
}
