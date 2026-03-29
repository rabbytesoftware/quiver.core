package step

import (
	"fmt"
	"time"

	dstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

type fetchHandler struct{}

// NewFetchHandler returns a Handler for fetch steps.
func NewFetchHandler() Handler {
	return &fetchHandler{}
}

func (h *fetchHandler) Validate(
	s models.RawStep,
) error {
	if s.URL == "" || s.To == "" {
		return fmt.Errorf("fetch step requires url and to")
	}

	return nil
}

func (h *fetchHandler) Build(
	s models.RawStep,
	timeout time.Duration,
	exitOnFailure bool,
) (dstep.Step, error) {
	return dstep.NewFetchStep(s.Title, s.URL, s.To, timeout, exitOnFailure), nil
}

func (h *fetchHandler) SupportsOverrides() bool { return false }
