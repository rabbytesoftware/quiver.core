// Package commands defines Asynx command implementations for port lifecycle events.
package commands

import (
	"strconv"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
)

// AllocatePort records a new port allocation.
type AllocatePort struct {
	Port      int
	Protocol  netbridge.Protocol
	OwnerKey  string
	Forwarded bool
}

// AggregateID returns the port number as the aggregate identifier.
func (c AllocatePort) AggregateID() string {
	return strconv.Itoa(c.Port)
}

// EventName returns the event name for this command.
func (c AllocatePort) EventName() string {
	return "port.Allocated"
}

// ShouldSnapshot is true even though ports churn with every execution. Under
// asynx v0.8 a snapshot is a single upserted row, not an appended one, so
// snapshotting each allocation costs O(1) per write instead of making every
// later read replay the port's whole allocate/deallocate history.
func (c AllocatePort) ShouldSnapshot() bool {
	return true
}

// Validate returns an error if the port is already actively allocated.
// A nil or zero-value current state is treated as available for allocation.
func (c AllocatePort) Validate(
	current *ports.PortAllocation,
) error {
	if current != nil && current.Port != 0 {
		return asynxModels.ErrValidation
	}
	return nil
}

// EmitEvent returns the new PortAllocation state produced by this command.
func (c AllocatePort) EmitEvent(
	_ *ports.PortAllocation,
) ports.PortAllocation {
	return ports.PortAllocation{
		Port:      c.Port,
		Protocol:  c.Protocol,
		OwnerKey:  c.OwnerKey,
		Forwarded: c.Forwarded,
	}
}
