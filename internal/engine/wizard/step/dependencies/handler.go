package dependencies

import (
	"context"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver/internal/engine/wizard/step"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) ShouldExecute(t domainstep.StepType) bool {
	return t == domainstep.StepTypeDependencies
}

func (h *Handler) Execute(_ context.Context, _ wizstep.Request, _ domainstep.Step) error {
	return nil
}
