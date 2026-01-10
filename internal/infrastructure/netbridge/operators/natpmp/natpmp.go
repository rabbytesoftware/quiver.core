package natpmp

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators/interfaces"
	"github.com/rabbytesoftware/quiver/internal/models/networking"
)

type NATPMP struct{}

func NewNATPMP() interfaces.OperatorInterface {
	return &NATPMP{}
}

func (n *NATPMP) Name() string {
	return "natpmp"
}

func (n *NATPMP) IsAvailable(context.Context) (bool, error) {
	return false, nil
}

func (n *NATPMP) IsPortAvailable(
	_ context.Context,
	_ int,
	_ networking.Protocol,
) (bool, error) {
	return false, fmt.Errorf("NAT-PMP not implemented")
}

func (n *NATPMP) IsProtocolAvailable(
	_ context.Context,
	_ networking.Protocol,
) (bool, error) {
	return false, fmt.Errorf("NAT-PMP not implemented")
}

func (n *NATPMP) ForwardRule(
	_ context.Context,
	_ networking.Rule,
) (networking.Port, error) {
	return networking.Port{}, fmt.Errorf("NAT-PMP not implemented")
}

func (n *NATPMP) ForwardPort(
	_ context.Context,
	_ networking.Port,
) (networking.Port, error) {
	return networking.Port{}, fmt.Errorf("NAT-PMP not implemented")
}

func (n *NATPMP) ReversePort(
	_ context.Context,
	_ networking.Port,
) (networking.Port, error) {
	return networking.Port{}, fmt.Errorf("NAT-PMP not implemented")
}

func (n *NATPMP) GetPortForwardingStatus(
	_ context.Context,
	_ networking.Port,
) (networking.ForwardingStatus, error) {
	return networking.ForwardingStatusError, fmt.Errorf("NAT-PMP not implemented")
}
