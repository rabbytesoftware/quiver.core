package commands

import (
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
)

func TestDeallocatePort_AggregateID(
	t *testing.T,
) {
	cmd := DeallocatePort{Port: 9090}
	assert.Equal(t, "9090", cmd.AggregateID())
}

func TestDeallocatePort_EventName(
	t *testing.T,
) {
	cmd := DeallocatePort{}
	assert.Equal(t, "port.Deallocated", cmd.EventName())
}

func TestDeallocatePort_ShouldSnapshot(
	t *testing.T,
) {
	cmd := DeallocatePort{}
	assert.False(t, cmd.ShouldSnapshot())
}

func TestDeallocatePort_Validate_NilCurrent(
	t *testing.T,
) {
	cmd := DeallocatePort{Port: 9090}
	assert.ErrorIs(t, cmd.Validate(nil), asynxModels.ErrValidation)
}

func TestDeallocatePort_Validate_ExistingAllocation(
	t *testing.T,
) {
	cmd := DeallocatePort{Port: 9090}
	current := &ports.PortAllocation{Port: 9090, OwnerKey: "owner-1"}
	assert.NoError(t, cmd.Validate(current))
}

func TestDeallocatePort_EmitEvent(
	t *testing.T,
) {
	cmd := DeallocatePort{Port: 9090}
	result := cmd.EmitEvent(&ports.PortAllocation{Port: 9090})

	assert.Equal(t, ports.PortAllocation{}, result)
}
