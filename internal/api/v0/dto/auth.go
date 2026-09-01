package dto

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// PairingCodeDTO is the one-time code an operator shows to pair a new device.
type PairingCodeDTO struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RedeemPairingCodeRequestDTO is submitted by quiver.desktop to exchange a
// pairing code for a session token.
type RedeemPairingCodeRequestDTO struct {
	Code     string `json:"code"`
	DeviceID string `json:"device_id"`
	Label    string `json:"label"`
}

// SessionTokenDTO carries the bearer token a device uses for every later
// request. It is shown exactly once — the daemon persists only its hash.
type SessionTokenDTO struct {
	Token string `json:"token"`
}

// DeviceDTO is one paired device, as returned by the admin device list.
type DeviceDTO struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	State      string    `json:"state"`
	PairedAt   time.Time `json:"paired_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// DeviceDTOFrom renders a domain device for the admin device list.
func DeviceDTOFrom(
	d auth.Device,
) DeviceDTO {
	return DeviceDTO{
		ID:         d.ID,
		Label:      d.Label,
		State:      string(d.State),
		PairedAt:   d.PairedAt,
		LastSeenAt: d.LastSeenAt,
	}
}
