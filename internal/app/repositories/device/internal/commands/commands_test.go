package commands

import (
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func TestPair_AggregateIDAndEventName(t *testing.T) {
	c := Pair{DeviceID: "dev-1"}
	assert.Equal(t, "dev-1", c.AggregateID())
	assert.Equal(t, "auth.device.paired.dev-1", c.EventName())
	assert.True(t, c.ShouldSnapshot())
}

func TestPair_Validate_NeverRejects(t *testing.T) {
	testCases := []struct {
		name    string
		current *auth.Device
	}{
		{name: "NewDevice", current: nil},
		{name: "AlreadyActive", current: &auth.Device{State: auth.DeviceStateActive}},
		{name: "PreviouslyRevoked", current: &auth.Device{State: auth.DeviceStateRevoked}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.NoError(t, Pair{DeviceID: "dev-1"}.Validate(tc.current))
		})
	}
}

func TestPair_EmitEvent(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	current := &auth.Device{ID: "dev-1", State: auth.DeviceStateRevoked, TokenHash: "old-hash"}

	got := Pair{DeviceID: "dev-1", Label: "laptop", TokenHash: "new-hash", Now: now}.EmitEvent(current)

	assert.Equal(t, auth.Device{
		ID:         "dev-1",
		Label:      "laptop",
		TokenHash:  "new-hash",
		State:      auth.DeviceStateActive,
		PairedAt:   now,
		LastSeenAt: now,
	}, got)
}

func TestRevoke_AggregateIDAndEventName(t *testing.T) {
	c := Revoke{DeviceID: "dev-1"}
	assert.Equal(t, "dev-1", c.AggregateID())
	assert.Equal(t, "auth.device.revoked.dev-1", c.EventName())
	assert.True(t, c.ShouldSnapshot())
}

func TestRevoke_Validate(t *testing.T) {
	testCases := []struct {
		name    string
		current *auth.Device
		wantErr bool
	}{
		{name: "UnknownDevice", current: nil, wantErr: true},
		{name: "Active", current: &auth.Device{State: auth.DeviceStateActive}, wantErr: false},
		{name: "AlreadyRevoked", current: &auth.Device{State: auth.DeviceStateRevoked}, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := Revoke{DeviceID: "dev-1"}.Validate(tc.current)
			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, asynxModels.ErrValidation)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestRevoke_EmitEvent_KeepsTokenHash(t *testing.T) {
	current := &auth.Device{ID: "dev-1", State: auth.DeviceStateActive, TokenHash: "hash"}

	got := Revoke{DeviceID: "dev-1"}.EmitEvent(current)

	assert.Equal(t, auth.DeviceStateRevoked, got.State)
	assert.Equal(t, "hash", got.TokenHash)
}

func TestTouch_AggregateIDAndEventName(t *testing.T) {
	c := Touch{DeviceID: "dev-1"}
	assert.Equal(t, "dev-1", c.AggregateID())
	assert.Equal(t, "auth.device.touched.dev-1", c.EventName())
	assert.True(t, c.ShouldSnapshot())
}

func TestTouch_Validate(t *testing.T) {
	testCases := []struct {
		name    string
		current *auth.Device
		wantErr bool
	}{
		{name: "UnknownDevice", current: nil, wantErr: true},
		{name: "Active", current: &auth.Device{State: auth.DeviceStateActive}, wantErr: false},
		{name: "Revoked", current: &auth.Device{State: auth.DeviceStateRevoked}, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := Touch{DeviceID: "dev-1"}.Validate(tc.current)
			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, asynxModels.ErrValidation)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestTouch_EmitEvent(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	current := &auth.Device{ID: "dev-1", State: auth.DeviceStateActive, LastSeenAt: now.Add(-time.Hour)}

	got := Touch{DeviceID: "dev-1", Now: now}.EmitEvent(current)

	assert.Equal(t, now, got.LastSeenAt)
}
