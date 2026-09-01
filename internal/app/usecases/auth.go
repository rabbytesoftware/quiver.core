package usecases

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/device"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/pairingcode"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

const sessionTokenBytes = 32

// AuthUsecase pairs quiver.desktop clients with the daemon when it is
// reachable over tcp://. It is never consulted when the daemon is bound to
// unix://, since a Unix socket connection is already trusted by filesystem
// permissions.
type AuthUsecase interface {
	// GeneratePairingCode mints a new one-time code. Reached only through the
	// loopback-gated admin path.
	GeneratePairingCode(
		ctx context.Context,
	) (code string, expiresAt time.Time, err error)

	// Redeem exchanges a valid pairing code for a session token, pairing a
	// device in the same call. The returned token is shown to the caller
	// exactly once — only its hash is ever persisted.
	Redeem(
		ctx context.Context,
		code string,
		deviceID string,
		label string,
	) (token string, err error)

	// Authenticate resolves a bearer token to its paired device, then updates
	// LastSeenAt in the background.
	Authenticate(
		ctx context.Context,
		rawToken string,
	) (auth.Device, error)

	ListDevices(
		ctx context.Context,
	) ([]auth.Device, error)
	RevokeDevice(
		ctx context.Context,
		deviceID string,
	) error
}

type authUsecase struct {
	pairingCode    pairingcode.PairingCode
	device         device.Device
	pairingCodeTTL time.Duration
}

// NewAuthUsecase constructs an AuthUsecase. pairingCodeTTL is read once by the
// caller per CLAUDE.md §15.2, not on every call.
func NewAuthUsecase(
	pc pairingcode.PairingCode,
	dev device.Device,
	pairingCodeTTL time.Duration,
) AuthUsecase {
	return &authUsecase{pairingCode: pc, device: dev, pairingCodeTTL: pairingCodeTTL}
}

func (u *authUsecase) GeneratePairingCode(
	ctx context.Context,
) (string, time.Time, error) {
	pc, err := u.pairingCode.Generate(ctx, u.pairingCodeTTL)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate pairing code: %w", err)
	}

	return pc.Code, pc.ExpiresAt, nil
}

func (u *authUsecase) Redeem(
	ctx context.Context,
	code string,
	deviceID string,
	label string,
) (string, error) {
	if err := u.pairingCode.Claim(ctx, code, deviceID, label); err != nil {
		return "", fmt.Errorf("redeem pairing code: %w", err)
	}

	token, hash, err := newSessionToken()
	if err != nil {
		return "", fmt.Errorf("redeem pairing code: %w", err)
	}

	if err := u.device.Pair(ctx, deviceID, label, hash); err != nil {
		return "", fmt.Errorf("redeem pairing code: %w", err)
	}

	return token, nil
}

func (u *authUsecase) Authenticate(
	ctx context.Context,
	rawToken string,
) (auth.Device, error) {
	d, err := u.device.Authenticate(ctx, hashToken(rawToken))
	if err != nil {
		return auth.Device{}, fmt.Errorf("authenticate: %w", err)
	}

	go u.touchInBackground(d.ID)

	return d, nil
}

// touchInBackground records LastSeenAt off the request path: it is
// bookkeeping the caller does not need to wait on, and a fresh context avoids
// its write being cancelled by the request that triggered it.
func (u *authUsecase) touchInBackground(
	deviceID string,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := u.device.Touch(ctx, deviceID); err != nil {
		slog.ErrorContext(ctx, "auth: touch device failed", "device", deviceID, "err", err)
	}
}

func (u *authUsecase) ListDevices(
	ctx context.Context,
) ([]auth.Device, error) {
	devices, err := u.device.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}

	return devices, nil
}

func (u *authUsecase) RevokeDevice(
	ctx context.Context,
	deviceID string,
) error {
	if err := u.device.Revoke(ctx, deviceID); err != nil {
		return fmt.Errorf("revoke device: %w", err)
	}

	return nil
}

// newSessionToken returns a fresh random bearer token and its SHA-256 hash.
// Only the hash is ever persisted.
func newSessionToken() (token string, hash string, err error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("new session token: %w", err)
	}

	token = hex.EncodeToString(raw)
	return token, hashToken(token), nil
}

func hashToken(
	token string,
) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
