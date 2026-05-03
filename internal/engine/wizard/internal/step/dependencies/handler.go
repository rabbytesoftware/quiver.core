package dependencies

import (
	"context"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/step"
)

// Executor resolves dependencies for a DependenciesStep.
// Injected at wizard construction time by the app layer.
type Executor func(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.DependenciesStep,
) error

type handler struct {
	exec Executor
}

// NewHandler returns a dependencies step handler backed by exec.
// If exec is nil the step is treated as a no-op.
func NewHandler(
	exec Executor,
) wizstep.Handler[domainstep.DependenciesStep] {
	return &handler{exec: exec}
}

func (h *handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.DependenciesStep,
) error {
	if h.exec == nil {
		return nil
	}
	return h.exec(ctx, req, s)
}
