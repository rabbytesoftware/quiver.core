package portable

import (
	"context"
	"fmt"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/portable/internal/install"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
)

type handler struct {
	installer install.Installer
}

func NewHandler(
	maxBytes int64,
) wizstep.Handler[domainstep.PortableStep] {
	return &handler{installer: install.New(unpack.New(maxBytes))}
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

	return h.installer.Install(stepCtx, install.Request{
		NSKey:   req.NSKey,
		WorkDir: req.WorkDir,
		From:    req.ResolvePath(s.From.Resolve(osArch)),
		To:      req.ResolvePath(s.To.Resolve(osArch)),
		Name:    s.Name,
	})
}
