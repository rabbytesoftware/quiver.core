// Package commands defines Asynx command implementations for port lifecycle events.
package commands

import (
	"strconv"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

// AllocatePort records a new port allocation.
type AllocatePort struct {
	Port      int
	Protocol  ports.Protocol
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

// ShouldSnapshot reports whether a snapshot should be written after this event.
func (c AllocatePort) ShouldSnapshot() bool {
	return false
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
