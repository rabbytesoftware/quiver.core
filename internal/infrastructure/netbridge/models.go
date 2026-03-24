package netbridge

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
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
	) (domain.PortRule, error)
	ForwardPorts(
		ctx context.Context,
		ports []int,
	) ([]domain.PortRule, error)

	ReversePort(
		ctx context.Context,
		port int,
	) (domain.PortRule, error)
	ReversePorts(
		ctx context.Context,
		ports []int,
	) ([]domain.PortRule, error)

	GetPortForwardingStatus(
		ctx context.Context,
		port int,
	) (domain.ForwardingStatus, error)
	GetPortForwardingStatuses(
		ctx context.Context,
		ports []int,
	) ([]domain.ForwardingStatus, error)
}
