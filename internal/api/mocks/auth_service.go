package mocks

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// AuthService is a hand-written stand-in for usecases.AuthUsecase.
type AuthService struct {
	GenerateCode      string
	GenerateExpiresAt time.Time
	GenerateErr       error

	RedeemToken string
	RedeemErr   error
	RedeemArgs  struct{ Code, DeviceID, Label string }

	AuthenticateResult auth.Device
	AuthenticateErr    error
	AuthenticateToken  string

	ListDevicesResult []auth.Device
	ListDevicesErr    error

	RevokeDeviceErr error
	RevokeDeviceID  string
}

func (m *AuthService) GeneratePairingCode(
	_ context.Context,
) (string, time.Time, error) {
	return m.GenerateCode, m.GenerateExpiresAt, m.GenerateErr
}

func (m *AuthService) Redeem(
	_ context.Context,
	code string,
	deviceID string,
	label string,
) (string, error) {
	m.RedeemArgs.Code = code
	m.RedeemArgs.DeviceID = deviceID
	m.RedeemArgs.Label = label
	return m.RedeemToken, m.RedeemErr
}

func (m *AuthService) Authenticate(
	_ context.Context,
	rawToken string,
) (auth.Device, error) {
	m.AuthenticateToken = rawToken
	return m.AuthenticateResult, m.AuthenticateErr
}

func (m *AuthService) ListDevices(
	_ context.Context,
) ([]auth.Device, error) {
	return m.ListDevicesResult, m.ListDevicesErr
}

func (m *AuthService) RevokeDevice(
	_ context.Context,
	deviceID string,
) error {
	m.RevokeDeviceID = deviceID
	return m.RevokeDeviceErr
}
