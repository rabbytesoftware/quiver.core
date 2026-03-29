package step

import (
	"fmt"
	"time"

	dstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

type signalHandler struct{}

// NewSignalHandler returns a Handler for signal steps.
func NewSignalHandler() Handler {
	return &signalHandler{}
}

func (h *signalHandler) Validate(
	s models.RawStep,
) error {
	if s.Signal == "" {
		return fmt.Errorf("signal step missing signal")
	}

	return nil
}

func (h *signalHandler) Build(
	s models.RawStep,
	timeout time.Duration,
	exitOnFailure bool,
) (dstep.Step, error) {
	return dstep.NewSignalStep(s.Title, s.Signal, timeout, exitOnFailure), nil
}

func (h *signalHandler) SupportsOverrides() bool { return false }
