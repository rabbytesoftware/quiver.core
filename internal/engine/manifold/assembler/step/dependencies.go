package step

import (
	"time"

	dstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

type dependenciesHandler struct{}

// NewDependenciesHandler returns a Handler for dependencies steps.
func NewDependenciesHandler() Handler {
	return &dependenciesHandler{}
}

func (h *dependenciesHandler) Validate(
	_ models.RawStep,
) error {
	return nil
}

func (h *dependenciesHandler) Build(
	s models.RawStep,
	_ time.Duration,
	_ bool,
) (dstep.Step, error) {
	return dstep.NewDependenciesStep(s.Title), nil
}

func (h *dependenciesHandler) SupportsOverrides() bool {
	return false
}
