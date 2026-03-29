package step

import (
	"fmt"
	"time"

	dstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

type runHandler struct{}

// NewRunHandler returns a Handler for run steps.
func NewRunHandler() Handler {
	return &runHandler{}
}

func (h *runHandler) Validate(
	s models.RawStep,
) error {
	if s.Command == "" {
		return fmt.Errorf("run step missing command")
	}

	return nil
}

func (h *runHandler) Build(
	s models.RawStep,
	timeout time.Duration,
	exitOnFailure bool,
) (dstep.Step, error) {
	return dstep.NewRunStep(s.Title, s.Command, timeout, exitOnFailure), nil
}

func (h *runHandler) SupportsOverrides() bool { return true }
