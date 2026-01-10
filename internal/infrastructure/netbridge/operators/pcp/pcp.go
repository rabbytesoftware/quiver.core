package pcp

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators/interfaces"
	"github.com/rabbytesoftware/quiver/internal/models/networking"
)

type PCP struct{}

func NewPCP() interfaces.OperatorInterface {
	return &PCP{}
}

func (p *PCP) Name() string {
	return "pcp"
}

func (p *PCP) IsAvailable(context.Context) (bool, error) {
	return false, nil
}

func (p *PCP) IsPortAvailable(
	_ context.Context,
	_ int,
	_ networking.Protocol,
) (bool, error) {
	return false, fmt.Errorf("PCP not implemented")
}

func (p *PCP) IsProtocolAvailable(
	_ context.Context,
	_ networking.Protocol,
) (bool, error) {
	return false, fmt.Errorf("PCP not implemented")
}

func (p *PCP) ForwardRule(
	_ context.Context,
	_ networking.Rule,
) (networking.Port, error) {
	return networking.Port{}, fmt.Errorf("PCP not implemented")
}

func (p *PCP) ForwardPort(
	_ context.Context,
	_ networking.Port,
) (networking.Port, error) {
	return networking.Port{}, fmt.Errorf("PCP not implemented")
}

func (p *PCP) ReversePort(
	_ context.Context,
	_ networking.Port,
) (networking.Port, error) {
	return networking.Port{}, fmt.Errorf("PCP not implemented")
}

func (p *PCP) GetPortForwardingStatus(
	_ context.Context,
	_ networking.Port,
) (networking.ForwardingStatus, error) {
	return networking.ForwardingStatusError, fmt.Errorf("PCP not implemented")
}
