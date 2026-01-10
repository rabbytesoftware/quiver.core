package holepunch

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators/interfaces"
	"github.com/rabbytesoftware/quiver/internal/models/networking"
)

type HolePunch struct{}

func NewHolePunch() interfaces.OperatorInterface {
	return &HolePunch{}
}

func (h *HolePunch) Name() string {
	return "holepunch"
}

func (h *HolePunch) IsAvailable(context.Context) (bool, error) {
	return false, nil
}

func (h *HolePunch) IsPortAvailable(
	_ context.Context,
	_ int,
	_ networking.Protocol,
) (bool, error) {
	return false, fmt.Errorf("hole punching not implemented")
}

func (h *HolePunch) IsProtocolAvailable(
	_ context.Context,
	_ networking.Protocol,
) (bool, error) {
	return false, fmt.Errorf("hole punching not implemented")
}

func (h *HolePunch) ForwardRule(
	_ context.Context,
	_ networking.Rule,
) (networking.Port, error) {
	return networking.Port{}, fmt.Errorf("hole punching not implemented")
}

func (h *HolePunch) ForwardPort(
	_ context.Context,
	_ networking.Port,
) (networking.Port, error) {
	return networking.Port{}, fmt.Errorf("hole punching not implemented")
}

func (h *HolePunch) ReversePort(
	_ context.Context,
	_ networking.Port,
) (networking.Port, error) {
	return networking.Port{}, fmt.Errorf("hole punching not implemented")
}

func (h *HolePunch) GetPortForwardingStatus(
	_ context.Context,
	_ networking.Port,
) (networking.ForwardingStatus, error) {
	return networking.ForwardingStatusError, fmt.Errorf("hole punching not implemented")
}
