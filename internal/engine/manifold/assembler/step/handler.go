package step

import (
	"time"

	dstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

// Handler validates and builds a domain step from a raw step model.
// Each step type has its own implementation registered in the Assembler.
type Handler interface {
	Validate(s models.RawStep) error
	Build(s models.RawStep, timeout time.Duration, exitOnFailure bool) (dstep.Step, error)
	SupportsOverrides() bool
}
