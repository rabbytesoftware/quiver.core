package commands

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// Pair (re)activates a device with a freshly issued token. It never rejects
// on prior state: a freshly redeemed pairing code is itself sufficient proof
// of authorization each time, so pairing an already-active or previously
// revoked device simply rotates its token rather than erroring.
type Pair struct {
	DeviceID  string
	Label     string
	TokenHash string
	Now       time.Time
}

func (c Pair) AggregateID() string {
	return c.DeviceID
}

func (c Pair) EventName() string {
	return "auth.device.paired." + c.DeviceID
}

func (c Pair) ShouldSnapshot() bool {
	return true
}

func (c Pair) Validate(
	_ *auth.Device,
) error {
	return nil
}

func (c Pair) EmitEvent(
	_ *auth.Device,
) auth.Device {
	return auth.Device{
		ID:         c.DeviceID,
		Label:      c.Label,
		TokenHash:  c.TokenHash,
		State:      auth.DeviceStateActive,
		PairedAt:   c.Now,
		LastSeenAt: c.Now,
	}
}
