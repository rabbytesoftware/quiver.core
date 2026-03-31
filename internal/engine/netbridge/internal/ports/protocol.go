// Package ports defines the domain types for port allocation.
package ports

// Protocol identifies the transport-layer protocol for a port allocation.
type Protocol string

const (
	ProtocolTCP    Protocol = "tcp"
	ProtocolUDP    Protocol = "udp"
	ProtocolTCPUDP Protocol = "tcp/udp"
)
