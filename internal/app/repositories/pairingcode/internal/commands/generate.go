package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// Generate creates a new pending pairing code. The aggregate ID is the code
// value itself, so this command only succeeds against a code that has never
// been issued before — a collision is retried by the caller with a fresh
// random code, this command never regenerates one itself.
type Generate struct {
	Code      string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (c Generate) AggregateID() string {
	return c.Code
}

func (c Generate) EventName() string {
	return "auth.pairingcode.generated." + c.Code
}

func (c Generate) ShouldSnapshot() bool {
	return true
}

func (c Generate) Validate(
	current *auth.PairingCode,
) error {
	if current != nil {
		return fmt.Errorf("generate pairing code: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c Generate) EmitEvent(
	_ *auth.PairingCode,
) auth.PairingCode {
	return auth.PairingCode{
		Code:      c.Code,
		State:     auth.PairingCodeStatePending,
		CreatedAt: c.CreatedAt,
		ExpiresAt: c.ExpiresAt,
	}
}
