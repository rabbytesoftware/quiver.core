package netbridge

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/models/port"
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
		protocol port.Protocol,
	) (bool, error)

	IsProtocolAvailable(
		ctx context.Context,
		protocol port.Protocol,
	) (bool, error)

	ForwardRule(
		ctx context.Context,
		rule port.Rule,
	) (port.Port, error)
	ForwardPort(
		ctx context.Context,
		port port.Port,
	) (port.Port, error)

	ReversePort(
		ctx context.Context,
		port port.Port,
	) (port.Port, error)

	GetPortForwardingStatus(
		ctx context.Context,
		port port.Port,
	) (port.ForwardingStatus, error)
}
