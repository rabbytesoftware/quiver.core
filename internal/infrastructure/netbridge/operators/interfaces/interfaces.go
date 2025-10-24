package interfaces

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/models/networking"
)

type OperatorInterface interface {
	Name() string

	IsAvailable(
		ctx context.Context,
	) (bool, error)

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
