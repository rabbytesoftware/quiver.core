package netbridge

import (
	"context"

	domainnetbridge "github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

type ForwardingStatus string

const (
	ForwardingStatusEnabled  ForwardingStatus = "enabled"
	ForwardingStatusDisabled ForwardingStatus = "disabled"
	ForwardingStatusError    ForwardingStatus = "error"
)

type PortRule struct {
	StartPort        int
	EndPort          int
	Protocol         domainnetbridge.Protocol
	ForwardingStatus ForwardingStatus
}

type NetbridgeInterface interface {
	IsEnabled() bool
	IsAvailable() bool

	PublicIP(
		ctx context.Context,
	) (string, error)
	LocalIP(
		ctx context.Context,
	) (string, error)

	IsPortAvailable(
		ctx context.Context,
		port int,
	) (bool, error)
	ArePortsAvailable(
		ctx context.Context,
		ports []int,
	) (bool, error)

	ForwardPort(
		ctx context.Context,
		port int,
	) (PortRule, error)
	ForwardPorts(
		ctx context.Context,
		ports []int,
	) ([]PortRule, error)

	ReversePort(
		ctx context.Context,
		port int,
	) (PortRule, error)
	ReversePorts(
		ctx context.Context,
		ports []int,
	) ([]PortRule, error)

	GetPortForwardingStatus(
		ctx context.Context,
		port int,
	) (ForwardingStatus, error)
	GetPortForwardingStatuses(
		ctx context.Context,
		ports []int,
	) ([]ForwardingStatus, error)
}
