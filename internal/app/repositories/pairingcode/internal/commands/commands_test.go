package commands

import (
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func TestGenerate_AggregateIDAndEventName(t *testing.T) {
	c := Generate{Code: "482913"}
	assert.Equal(t, "482913", c.AggregateID())
	assert.Equal(t, "auth.pairingcode.generated.482913", c.EventName())
	assert.True(t, c.ShouldSnapshot())
}

func TestGenerate_Validate(t *testing.T) {
	testCases := []struct {
		name    string
		current *auth.PairingCode
		wantErr bool
	}{
		{name: "NoExistingCode", current: nil, wantErr: false},
		{name: "CodeAlreadyExists", current: &auth.PairingCode{Code: "482913"}, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := Generate{Code: "482913"}.Validate(tc.current)
			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, asynxModels.ErrValidation)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestGenerate_EmitEvent(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	expires := now.Add(5 * time.Minute)

	got := Generate{Code: "482913", CreatedAt: now, ExpiresAt: expires}.EmitEvent(nil)

	assert.Equal(t, auth.PairingCode{
		Code:      "482913",
		State:     auth.PairingCodeStatePending,
		CreatedAt: now,
		ExpiresAt: expires,
	}, got)
}

func TestClaim_AggregateIDAndEventName(t *testing.T) {
	c := Claim{Code: "482913"}
	assert.Equal(t, "482913", c.AggregateID())
	assert.Equal(t, "auth.pairingcode.claimed.482913", c.EventName())
	assert.True(t, c.ShouldSnapshot())
}

func TestClaim_Validate(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	testCases := []struct {
		name    string
		current *auth.PairingCode
		wantErr bool
	}{
		{name: "NoSuchCode", current: nil, wantErr: true},
		{
			name:    "PendingAndClaimable",
			current: &auth.PairingCode{State: auth.PairingCodeStatePending, ExpiresAt: now.Add(time.Minute)},
			wantErr: false,
		},
		{
			name:    "AlreadyClaimed",
			current: &auth.PairingCode{State: auth.PairingCodeStateClaimed, ExpiresAt: now.Add(time.Minute)},
			wantErr: true,
		},
		{
			name:    "Expired",
			current: &auth.PairingCode{State: auth.PairingCodeStatePending, ExpiresAt: now.Add(-time.Minute)},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := Claim{Code: "482913", Now: now}.Validate(tc.current)
			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, asynxModels.ErrValidation)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestClaim_EmitEvent(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	current := &auth.PairingCode{
		Code:      "482913",
		State:     auth.PairingCodeStatePending,
		CreatedAt: now,
		ExpiresAt: now.Add(time.Minute),
	}

	got := Claim{Code: "482913", DeviceID: "dev-1", Label: "laptop", Now: now}.EmitEvent(current)

	assert.Equal(t, auth.PairingCodeStateClaimed, got.State)
	assert.Equal(t, "482913", got.Code)
	assert.Equal(t, current.CreatedAt, got.CreatedAt)
	assert.Equal(t, current.ExpiresAt, got.ExpiresAt)
}
