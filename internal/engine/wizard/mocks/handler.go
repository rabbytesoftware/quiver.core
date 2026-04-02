package mocks

import (
	"context"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
)

// Handler is a mock for wizstep.Handler.
// Set ShouldExecuteFunc to control which step types this handler claims.
// Set ExecuteFunc to control the execution result.
type Handler struct {
	ShouldExecuteFunc func(t domainstep.StepType) bool
	ExecuteFunc       func(ctx context.Context, req wizstep.Request, s domainstep.Step) error
}

func (h *Handler) ShouldExecute(t domainstep.StepType) bool {
	if h.ShouldExecuteFunc != nil {
		return h.ShouldExecuteFunc(t)
	}
	return false
}

func (h *Handler) Execute(
	ctx context.Context,
	req wizstep.Request,
	s domainstep.Step,
) error {
	if h.ExecuteFunc != nil {
		return h.ExecuteFunc(ctx, req, s)
	}
	return nil
}
