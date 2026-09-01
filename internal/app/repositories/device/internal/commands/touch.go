package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// Touch stamps LastSeenAt on an active device. Sent from a background
// goroutine after a successful Authenticate, so it never blocks the request
// that triggered it.
type Touch struct {
	DeviceID string
	Now      time.Time
}

func (c Touch) AggregateID() string {
	return c.DeviceID
}

func (c Touch) EventName() string {
	return "auth.device.touched." + c.DeviceID
}

func (c Touch) ShouldSnapshot() bool {
	return true
}

func (c Touch) Validate(
	current *auth.Device,
) error {
	if current == nil || current.State != auth.DeviceStateActive {
		return fmt.Errorf("touch device: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Touch) EmitEvent(
	current *auth.Device,
) auth.Device {
	updated := *current
	updated.LastSeenAt = c.Now
	return updated
}
