package pairingcode

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/char2cs/asynx"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	pairingcodecmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/pairingcode/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

const (
	codeDigits           = 6
	maxGenerateAttempts  = 5
	pairingCodeUpperExcl = 1_000_000 // 10^codeDigits
)

// PairingCode issues and redeems one-time device-pairing codes. It has no read
// model: the code value is the only thing anyone ever looks it up by, and
// Asynx already indexes aggregates by ID.
type PairingCode interface {
	// Generate mints a new pending code that expires after ttl.
	Generate(
		ctx context.Context,
		ttl time.Duration,
	) (auth.PairingCode, error)

	// Claim redeems a pending, unexpired code. deviceID and label are recorded
	// on the event as an audit trail; the pairing code aggregate itself holds
	// no device state.
	Claim(
		ctx context.Context,
		code string,
		deviceID string,
		label string,
	) error

	Shutdown(ctx context.Context) error
}

type repoOpts struct {
	generateCode func() (string, error)
}

// Option configures New.
type Option func(*repoOpts)

// WithCodeGenerator overrides the random-code source. Tests use this to force
// a collision deterministically instead of relying on crypto/rand chance.
func WithCodeGenerator(fn func() (string, error)) Option {
	return func(o *repoOpts) { o.generateCode = fn }
}

type pairingCodeRepo struct {
	ax           asynx.Asynx[auth.PairingCode]
	generateCode func() (string, error)
}

// New constructs a PairingCode repository backed by its own Asynx instance.
func New(
	ax asynx.Asynx[auth.PairingCode],
	opts ...Option,
) (PairingCode, error) {
	if ax == nil {
		return nil, fmt.Errorf("pairingcode repository: asynx instance is required")
	}

	cfg := repoOpts{generateCode: randomCode}
	for _, o := range opts {
		o(&cfg)
	}

	return &pairingCodeRepo{ax: ax, generateCode: cfg.generateCode}, nil
}

func (r *pairingCodeRepo) Generate(
	ctx context.Context,
	ttl time.Duration,
) (auth.PairingCode, error) {
	now := time.Now()

	for attempt := 0; attempt < maxGenerateAttempts; attempt++ {
		code, err := r.generateCode()
		if err != nil {
			return auth.PairingCode{}, fmt.Errorf("generate pairing code: random code: %w", err)
		}

		evt, sendErr := r.ax.SendWait(ctx, pairingcodecmds.Generate{
			Code:      code,
			CreatedAt: now,
			ExpiresAt: now.Add(ttl),
		})
		if sendErr == nil {
			return evt.Aggregate, nil
		}
	}

	return auth.PairingCode{}, fmt.Errorf(
		"generate pairing code: %d attempts exhausted: %w", maxGenerateAttempts, apperrors.ErrStateViolation,
	)
}

func (r *pairingCodeRepo) Claim(
	ctx context.Context,
	code string,
	deviceID string,
	label string,
) error {
	_, err := r.ax.SendWait(ctx, pairingcodecmds.Claim{
		Code:     code,
		DeviceID: deviceID,
		Label:    label,
		Now:      time.Now(),
	})
	if err != nil {
		return fmt.Errorf("claim pairing code: %w", apperrors.ErrInvalidPairingCode)
	}

	return nil
}

func (r *pairingCodeRepo) Shutdown(
	ctx context.Context,
) error {
	return r.ax.Shutdown(ctx)
}

// randomCode returns a cryptographically random codeDigits-digit string,
// zero-padded.
func randomCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(pairingCodeUpperExcl))
	if err != nil {
		return "", fmt.Errorf("random code: %w", err)
	}

	return fmt.Sprintf("%0*d", codeDigits, n.Int64()), nil
}
