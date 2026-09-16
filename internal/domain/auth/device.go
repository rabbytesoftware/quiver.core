package auth

import "time"

// DeviceState is the lifecycle of a paired device.
type DeviceState string

const (
	DeviceStateActive  DeviceState = "active"
	DeviceStateRevoked DeviceState = "revoked"
)

// Device is one quiver.desktop client that redeemed a pairing code. TokenHash
// is a SHA-256 digest of the bearer token issued at pairing time; the raw
// token itself is never persisted.
type Device struct {
	ID         string      `yaml:"id"           json:"id"`
	Label      string      `yaml:"label"        json:"label"`
	TokenHash  string      `yaml:"token_hash"   json:"token_hash"`
	State      DeviceState `yaml:"state"        json:"state"`
	PairedAt   time.Time   `yaml:"paired_at"    json:"paired_at"`
	LastSeenAt time.Time   `yaml:"last_seen_at" json:"last_seen_at"`
}

// IsActive reports whether the device's credential is currently usable.
func (d Device) IsActive() bool {
	return d.State == DeviceStateActive
}
