package netbridge

import (
	"context"

	netbridgeModels "github.com/rabbytesoftware/quiver/internal/models/netbridge"
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
) (bool, error) {
	return true, nil
}

func (n *NetbridgeImpl) ArePortsAvailable(
	ctx context.Context,
	ports []int,
) (bool, error) {
	return true, nil
}

func (n *NetbridgeImpl) ForwardPort(
	ctx context.Context,
	portNum int,
) (netbridgeModels.PortRule, error) {
	return netbridgeModels.PortRule{
		StartPort:        portNum,
		EndPort:          portNum,
		Protocol:         netbridgeModels.ProtocolTCP,
		ForwardingStatus: netbridgeModels.ForwardingStatusEnabled,
	}, nil
}

func (n *NetbridgeImpl) ForwardPorts(
	ctx context.Context,
	ports []int,
) ([]netbridgeModels.PortRule, error) {
	return []netbridgeModels.PortRule{}, nil
}

func (n *NetbridgeImpl) ReversePort(
	ctx context.Context,
	portNum int,
) (netbridgeModels.PortRule, error) {
	return netbridgeModels.PortRule{
		StartPort:        portNum,
		EndPort:          portNum,
		Protocol:         netbridgeModels.ProtocolTCP,
		ForwardingStatus: netbridgeModels.ForwardingStatusDisabled,
	}, nil
}

func (n *NetbridgeImpl) ReversePorts(
	ctx context.Context,
	ports []int,
) ([]netbridgeModels.PortRule, error) {
	return []netbridgeModels.PortRule{}, nil
}

func (n *NetbridgeImpl) GetPortForwardingStatus(
	ctx context.Context,
	portNum int,
) (netbridgeModels.ForwardingStatus, error) {
	return netbridgeModels.ForwardingStatusEnabled, nil
}

func (n *NetbridgeImpl) GetPortForwardingStatuses(
	ctx context.Context,
	ports []int,
) ([]netbridgeModels.ForwardingStatus, error) {
	return []netbridgeModels.ForwardingStatus{}, nil
}
