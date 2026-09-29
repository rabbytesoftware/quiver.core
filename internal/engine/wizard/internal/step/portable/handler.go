package portable

import (
	"context"
	"fmt"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable/internal/install"
)

type handler struct {
	installer install.Installer
}

func NewHandler(
	maxBytes int64,
) wizstep.Handler[domainstep.PortableStep] {
	return &handler{installer: install.New(maxBytes)}
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.PortableStep,
) error {
	osArch := req.OSArch.String()

	stepCtx, cancel, err := wizstep.WithTimeout(ctx, s.Timeout.Resolve(osArch))
	if err != nil {
		return fmt.Errorf("portable: %w", err)
	}
	defer cancel()

	from := req.ResolvePath(s.From.Resolve(osArch))
	to := req.ResolvePath(s.To.Resolve(osArch))

	apps, output, err := h.installer.Place(stepCtx, req.OSArch, req.WorkDir, from, to, s.Name)
	if err != nil {
		return err
	}

	if err := h.installer.Record(stepCtx, req.NSKey, req.WorkDir, apps); err != nil {
		return err
	}

	return h.installer.RemoveSource(req.WorkDir, from, output)
}
