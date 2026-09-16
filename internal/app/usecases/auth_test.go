package usecases_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func TestAuthUsecase_GeneratePairingCode_ReturnsCodeAndExpiry(t *testing.T) {
	want := auth.PairingCode{Code: "482913", ExpiresAt: time.Now().Add(5 * time.Minute)}
	pc := &mocks.MockPairingCode{
		GenerateFn: func(ctx context.Context, ttl time.Duration) (auth.PairingCode, error) {
			assert.Equal(t, 5*time.Minute, ttl)
			return want, nil
		},
	}

	u := usecases.NewAuthUsecase(pc, &mocks.MockDevice{}, 5*time.Minute)

	code, expiresAt, err := u.GeneratePairingCode(context.Background())
	require.NoError(t, err)
	assert.Equal(t, want.Code, code)
	assert.Equal(t, want.ExpiresAt, expiresAt)
}

func TestAuthUsecase_GeneratePairingCode_RepoFails_ReturnsError(t *testing.T) {
	pc := &mocks.MockPairingCode{
		GenerateFn: func(ctx context.Context, ttl time.Duration) (auth.PairingCode, error) {
			return auth.PairingCode{}, apperrors.ErrStateViolation
		},
	}

	u := usecases.NewAuthUsecase(pc, &mocks.MockDevice{}, time.Minute)

	_, _, err := u.GeneratePairingCode(context.Background())
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestAuthUsecase_Redeem_Success_PairsDeviceAndReturnsToken(t *testing.T) {
	var claimedCode, claimedDevice, claimedLabel string
	var pairedDevice, pairedLabel, pairedHash string

	pc := &mocks.MockPairingCode{
		ClaimFn: func(ctx context.Context, code, deviceID, label string) error {
			claimedCode, claimedDevice, claimedLabel = code, deviceID, label
			return nil
		},
	}
	dev := &mocks.MockDevice{
		PairFn: func(ctx context.Context, deviceID, label, tokenHash string) error {
			pairedDevice, pairedLabel, pairedHash = deviceID, label, tokenHash
			return nil
		},
	}

	u := usecases.NewAuthUsecase(pc, dev, time.Minute)

	token, err := u.Redeem(context.Background(), "482913", "dev-1", "laptop")
	require.NoError(t, err)

	assert.Equal(t, "482913", claimedCode)
	assert.Equal(t, "dev-1", claimedDevice)
	assert.Equal(t, "laptop", claimedLabel)

	assert.Equal(t, "dev-1", pairedDevice)
	assert.Equal(t, "laptop", pairedLabel)
	assert.NotEmpty(t, token)
	assert.NotEmpty(t, pairedHash)
	assert.NotEqual(t, token, pairedHash, "the raw token must never be the value that gets persisted")
}

func TestAuthUsecase_Redeem_InvalidCode_NeverPairsDevice(t *testing.T) {
	pc := &mocks.MockPairingCode{
		ClaimFn: func(ctx context.Context, code, deviceID, label string) error {
			return apperrors.ErrInvalidPairingCode
		},
	}
	paired := false
	dev := &mocks.MockDevice{
		PairFn: func(ctx context.Context, deviceID, label, tokenHash string) error {
			paired = true
			return nil
		},
	}

	u := usecases.NewAuthUsecase(pc, dev, time.Minute)

	_, err := u.Redeem(context.Background(), "000000", "dev-1", "laptop")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidPairingCode)
	assert.False(t, paired)
}

func TestAuthUsecase_Redeem_PairFails_ReturnsError(t *testing.T) {
	pc := &mocks.MockPairingCode{}
	dev := &mocks.MockDevice{
		PairFn: func(ctx context.Context, deviceID, label, tokenHash string) error {
			return apperrors.ErrStateViolation
		},
	}

	u := usecases.NewAuthUsecase(pc, dev, time.Minute)

	_, err := u.Redeem(context.Background(), "482913", "dev-1", "laptop")
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestAuthUsecase_Authenticate_Success_TouchesInBackground(t *testing.T) {
	touched := make(chan string, 1)
	dev := &mocks.MockDevice{
		AuthenticateFn: func(ctx context.Context, tokenHash string) (auth.Device, error) {
			return auth.Device{ID: "dev-1", State: auth.DeviceStateActive}, nil
		},
		TouchFn: func(ctx context.Context, deviceID string) error {
			touched <- deviceID
			return nil
		},
	}

	u := usecases.NewAuthUsecase(&mocks.MockPairingCode{}, dev, time.Minute)

	d, err := u.Authenticate(context.Background(), "raw-token")
	require.NoError(t, err)
	assert.Equal(t, "dev-1", d.ID)

	select {
	case id := <-touched:
		assert.Equal(t, "dev-1", id)
	case <-time.After(time.Second):
		t.Fatal("touch was not called in the background")
	}
}

func TestAuthUsecase_Authenticate_Fails_ReturnsUnauthorized(t *testing.T) {
	dev := &mocks.MockDevice{
		AuthenticateFn: func(ctx context.Context, tokenHash string) (auth.Device, error) {
			return auth.Device{}, apperrors.ErrUnauthorized
		},
	}

	u := usecases.NewAuthUsecase(&mocks.MockPairingCode{}, dev, time.Minute)

	_, err := u.Authenticate(context.Background(), "bad-token")
	assert.ErrorIs(t, err, apperrors.ErrUnauthorized)
}

func TestAuthUsecase_Authenticate_TouchFailure_DoesNotFailTheCall(t *testing.T) {
	dev := &mocks.MockDevice{
		AuthenticateFn: func(ctx context.Context, tokenHash string) (auth.Device, error) {
			return auth.Device{ID: "dev-1", State: auth.DeviceStateActive}, nil
		},
		TouchFn: func(ctx context.Context, deviceID string) error {
			return errors.New("touch failed")
		},
	}

	u := usecases.NewAuthUsecase(&mocks.MockPairingCode{}, dev, time.Minute)

	_, err := u.Authenticate(context.Background(), "raw-token")
	assert.NoError(t, err)
}

func TestAuthUsecase_ListDevices(t *testing.T) {
	want := []auth.Device{{ID: "dev-1"}, {ID: "dev-2"}}
	dev := &mocks.MockDevice{
		ListFn: func(ctx context.Context) ([]auth.Device, error) {
			return want, nil
		},
	}

	u := usecases.NewAuthUsecase(&mocks.MockPairingCode{}, dev, time.Minute)

	got, err := u.ListDevices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestAuthUsecase_ListDevices_RepoFails_ReturnsError(t *testing.T) {
	dev := &mocks.MockDevice{
		ListFn: func(ctx context.Context) ([]auth.Device, error) {
			return nil, apperrors.ErrStateViolation
		},
	}

	u := usecases.NewAuthUsecase(&mocks.MockPairingCode{}, dev, time.Minute)

	_, err := u.ListDevices(context.Background())
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestAuthUsecase_RevokeDevice(t *testing.T) {
	revoked := ""
	dev := &mocks.MockDevice{
		RevokeFn: func(ctx context.Context, deviceID string) error {
			revoked = deviceID
			return nil
		},
	}

	u := usecases.NewAuthUsecase(&mocks.MockPairingCode{}, dev, time.Minute)

	require.NoError(t, u.RevokeDevice(context.Background(), "dev-1"))
	assert.Equal(t, "dev-1", revoked)
}

func TestAuthUsecase_RevokeDevice_RepoFails_ReturnsError(t *testing.T) {
	dev := &mocks.MockDevice{
		RevokeFn: func(ctx context.Context, deviceID string) error {
			return apperrors.ErrNotFound
		},
	}

	u := usecases.NewAuthUsecase(&mocks.MockPairingCode{}, dev, time.Minute)

	err := u.RevokeDevice(context.Background(), "missing")
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}
