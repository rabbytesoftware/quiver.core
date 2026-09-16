package auth

import "time"

// PairingCodeState is the lifecycle of a one-time pairing code.
type PairingCodeState string

const (
	PairingCodeStatePending PairingCodeState = "pending"
	PairingCodeStateClaimed PairingCodeState = "claimed"
)

// PairingCode is a short-lived, single-use code that authorizes exactly one
// device to redeem a session token. It exists only while the daemon is
// reachable over tcp:// — connections over the Unix socket are already
// trusted and never generate or redeem one.
type PairingCode struct {
	Code      string           `yaml:"code"       json:"code"`
	State     PairingCodeState `yaml:"state"      json:"state"`
	CreatedAt time.Time        `yaml:"created_at" json:"created_at"`
	ExpiresAt time.Time        `yaml:"expires_at" json:"expires_at"`
}

// CanClaim reports whether the code is still pending and has not expired as of now.
func (p PairingCode) CanClaim(now time.Time) bool {
	return p.State == PairingCodeStatePending && now.Before(p.ExpiresAt)
}
