package dto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func TestDeviceDTOFrom(t *testing.T) {
	now := time.Now()
	d := auth.Device{
		ID: "dev-1", Label: "laptop", State: auth.DeviceStateActive,
		PairedAt: now, LastSeenAt: now,
	}

	got := DeviceDTOFrom(d)

	assert.Equal(t, "dev-1", got.ID)
	assert.Equal(t, "laptop", got.Label)
	assert.Equal(t, "active", got.State)
	assert.Equal(t, now, got.PairedAt)
	assert.Equal(t, now, got.LastSeenAt)
}
