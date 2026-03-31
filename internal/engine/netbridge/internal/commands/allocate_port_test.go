package commands

import (
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

func TestAllocatePort_AggregateID(
	t *testing.T,
) {
	cmd := AllocatePort{Port: 8080}
	assert.Equal(t, "8080", cmd.AggregateID())
}

func TestAllocatePort_EventName(
	t *testing.T,
) {
	cmd := AllocatePort{}
	assert.Equal(t, "port.Allocated", cmd.EventName())
}

func TestAllocatePort_ShouldSnapshot(
	t *testing.T,
) {
	cmd := AllocatePort{}
	assert.False(t, cmd.ShouldSnapshot())
}

func TestAllocatePort_Validate_NilCurrent(
	t *testing.T,
) {
	cmd := AllocatePort{Port: 8080}
	assert.NoError(t, cmd.Validate(nil))
}

func TestAllocatePort_Validate_ZeroPortCurrent(
	t *testing.T,
) {
	cmd := AllocatePort{Port: 8080}
	current := &ports.PortAllocation{Port: 0}
	assert.NoError(t, cmd.Validate(current))
}

func TestAllocatePort_Validate_AlreadyAllocated(
	t *testing.T,
) {
	cmd := AllocatePort{Port: 8080}
	current := &ports.PortAllocation{Port: 8080, OwnerKey: "existing-owner"}
	assert.ErrorIs(t, cmd.Validate(current), asynxModels.ErrValidation)
}

func TestAllocatePort_EmitEvent(
	t *testing.T,
) {
	cmd := AllocatePort{
		Port:      8080,
		Protocol:  ports.ProtocolTCP,
		OwnerKey:  "owner-1",
		Forwarded: true,
	}

	result := cmd.EmitEvent(nil)

	assert.Equal(t, 8080, result.Port)
	assert.Equal(t, ports.ProtocolTCP, result.Protocol)
	assert.Equal(t, "owner-1", result.OwnerKey)
	assert.True(t, result.Forwarded)
}
