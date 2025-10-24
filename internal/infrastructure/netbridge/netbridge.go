package netbridge

import (
	"context"

	domain "github.com/rabbytesoftware/quiver/internal/models/networking"
)

// TODO: We should implement an statregy pattern for the netbridge
// TODO: so we can easily add new implementations and support multiple protocols
// TODO: like UPnP, NAT-PMP, hole-punching, etc.

type NetbridgeImpl struct {
}

func NewNetbridge() NetbridgeInterface {
	return &NetbridgeImpl{}
}

func (n *NetbridgeImpl) IsEnabled() bool {
	return true
}

func (n *NetbridgeImpl) IsAvailable() bool {
	return true
}

func (n *NetbridgeImpl) PublicIP(
	ctx context.Context,
) (string, error) {
	return "", nil
}

func (n *NetbridgeImpl) LocalIP(
	ctx context.Context,
) (string, error) {
	return "", nil
}

func (n *NetbridgeImpl) IsPortAvailable(
	ctx context.Context,
	port int,
	protocol domain.Protocol,
) (bool, error) {
	return true, nil
}

func (n *NetbridgeImpl) IsProtocolAvailable(
	ctx context.Context,
	protocol domain.Protocol,
) (bool, error) {
	return true, nil
}

func (n *NetbridgeImpl) ForwardRule(
	ctx context.Context,
	rule domain.Rule,
) (domain.Port, error) {
	return domain.Port{}, nil
}

func (n *NetbridgeImpl) ForwardPort(
	ctx context.Context,
	port domain.Port,
) (domain.Port, error) {
	return domain.Port{
		StartPort:        port.StartPort,
		EndPort:          port.EndPort,
		Protocol:         port.Protocol,
		ForwardingStatus: port.ForwardingStatus,
	}, nil
}

func (n *NetbridgeImpl) ReversePort(
	ctx context.Context,
	port domain.Port,
) (domain.Port, error) {
	return domain.Port{
		StartPort:        port.StartPort,
		EndPort:          port.EndPort,
		Protocol:         port.Protocol,
		ForwardingStatus: port.ForwardingStatus,
	}, nil
}

func (n *NetbridgeImpl) GetPortForwardingStatus(
	ctx context.Context,
	port domain.Port,
) (domain.ForwardingStatus, error) {
	return port.ForwardingStatus, nil
}
