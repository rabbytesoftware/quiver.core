package netbridge

import (
	"context"

	netbridgeModels "github.com/rabbytesoftware/quiver/internal/models/netbridge"
)

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
	) (netbridgeModels.PortRule, error)
	ForwardPorts(
		ctx context.Context,
		ports []int,
	) ([]netbridgeModels.PortRule, error)

	ReversePort(
		ctx context.Context,
		port int,
	) (netbridgeModels.PortRule, error)
	ReversePorts(
		ctx context.Context,
		ports []int,
	) ([]netbridgeModels.PortRule, error)

	GetPortForwardingStatus(
		ctx context.Context,
		port int,
	) (netbridgeModels.ForwardingStatus, error)
	GetPortForwardingStatuses(
		ctx context.Context,
		ports []int,
	) ([]netbridgeModels.ForwardingStatus, error)
}
