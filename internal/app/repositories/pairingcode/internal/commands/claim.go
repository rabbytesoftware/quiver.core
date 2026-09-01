package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// Claim marks a pending, unexpired pairing code as redeemed. DeviceID and
// Label travel with the event as an audit trail only — the device aggregate
// itself is created by a separate command in the device repository once this
// one succeeds.
type Claim struct {
	Code     string
	DeviceID string
	Label    string
	Now      time.Time
}

func (c Claim) AggregateID() string {
	return c.Code
}

func (c Claim) EventName() string {
	return "auth.pairingcode.claimed." + c.Code
}

func (c Claim) ShouldSnapshot() bool {
	return true
}

func (c Claim) Validate(
	current *auth.PairingCode,
) error {
	if current == nil || !current.CanClaim(c.Now) {
		return fmt.Errorf("claim pairing code: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Claim) EmitEvent(
	current *auth.PairingCode,
) auth.PairingCode {
	updated := *current
	updated.State = auth.PairingCodeStateClaimed
	return updated
}
