package commands

import (
	"strconv"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
)

// DeallocatePort removes a port allocation.
type DeallocatePort struct {
	Port int
}

// AggregateID returns the port number as the aggregate identifier.
func (c DeallocatePort) AggregateID() string {
	return strconv.Itoa(c.Port)
}

// EventName returns the event name for this command.
func (c DeallocatePort) EventName() string {
	return "port.Deallocated"
}

// ShouldSnapshot is true even though ports churn with every execution. Under
// asynx v0.8 a snapshot is a single upserted row, not an appended one, so
// recording the release costs O(1) per write and keeps a freed port one read
// away instead of a replay of its whole allocate/deallocate history.
func (c DeallocatePort) ShouldSnapshot() bool {
	return true
}

// Validate returns an error if no allocation exists for this port.
func (c DeallocatePort) Validate(
	current *ports.PortAllocation,
) error {
	if current == nil {
		return asynxModels.ErrValidation
	}
	return nil
}

// EmitEvent returns the zero-value PortAllocation, clearing the aggregate state.
func (c DeallocatePort) EmitEvent(
	_ *ports.PortAllocation,
) ports.PortAllocation {
	return ports.PortAllocation{}
}
