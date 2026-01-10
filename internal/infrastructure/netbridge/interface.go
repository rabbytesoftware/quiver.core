package netbridge

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/models/networking"
)

type NetbridgeInterface interface {
	IsEnabled() bool
	IsAvailable() bool
	DetectInfrastructure(
		ctx context.Context,
	) (InfrastructureStatus, error)

	PublicIP(
		ctx context.Context,
	) (string, error)
	LocalIP(
		ctx context.Context,
	) (string, error)

	IsPortAvailable(
		ctx context.Context,
		port int,
		protocol networking.Protocol,
	) (bool, error)

	IsProtocolAvailable(
		ctx context.Context,
		protocol networking.Protocol,
	) (bool, error)

	ForwardRule(
		ctx context.Context,
		rule networking.Rule,
	) (networking.Port, error)
	ForwardPort(
		ctx context.Context,
		port networking.Port,
	) (networking.Port, error)

	ReversePort(
		ctx context.Context,
		port networking.Port,
	) (networking.Port, error)

	GetPortForwardingStatus(
		ctx context.Context,
		port networking.Port,
	) (networking.ForwardingStatus, error)
}
