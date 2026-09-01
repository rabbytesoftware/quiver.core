package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// Revoke deactivates an active device's credential. TokenHash is left as-is:
// State gates whether Authenticate accepts it, and clearing the column would
// need every revoked device to share one placeholder value, colliding on the
// store's unique index over token_hash.
type Revoke struct {
	DeviceID string
}

func (c Revoke) AggregateID() string {
	return c.DeviceID
}

func (c Revoke) EventName() string {
	return "auth.device.revoked." + c.DeviceID
}

func (c Revoke) ShouldSnapshot() bool {
	return true
}

func (c Revoke) Validate(
	current *auth.Device,
) error {
	if current == nil || current.State != auth.DeviceStateActive {
		return fmt.Errorf("revoke device: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Revoke) EmitEvent(
	current *auth.Device,
) auth.Device {
	updated := *current
	updated.State = auth.DeviceStateRevoked
	return updated
}
