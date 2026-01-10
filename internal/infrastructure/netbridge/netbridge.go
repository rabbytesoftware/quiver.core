package netbridge

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/netbridge/operators/interfaces"
	domain "github.com/rabbytesoftware/quiver/internal/models/networking"
)

// TODO: We should implement an statregy pattern for the netbridge
// TODO: so we can easily add new implementations and support multiple protocols
// TODO: like UPnP, NAT-PMP, hole-punching, etc.

type NetbridgeImpl struct {
	operators *operators.OperatorContainer
	operator  interfaces.OperatorInterface
}

func NewNetbridge() NetbridgeInterface {
	return &NetbridgeImpl{
		operators: operators.NewOperatorContainer(),
	}
}

func (n *NetbridgeImpl) IsEnabled() bool {
	return config.GetNetbridge().Enabled
}

func (n *NetbridgeImpl) IsAvailable() bool {
	if !n.IsEnabled() {
		return false
	}

	operator := n.obtainOperator(context.Background())
	return operator != nil
}

func (n *NetbridgeImpl) DetectInfrastructure(
	ctx context.Context,
) (InfrastructureStatus, error) {
	status := InfrastructureStatus{
		Enabled: n.IsEnabled(),
	}

	if !status.Enabled {
		return status, nil
	}

	if n.operators == nil {
		n.operators = operators.NewOperatorContainer()
	}

	status.Available = n.operators.AvailableNames(ctx)
	operator := n.obtainOperator(ctx)
	if operator != nil {
		status.Selected = operator.Name()
	}
	status.HasFallback = len(status.Available) > 1

	return status, nil
}

func (n *NetbridgeImpl) PublicIP(
	ctx context.Context,
) (string, error) {
	if !n.IsEnabled() {
		return "", fmt.Errorf("netbridge disabled")
	}

	operator := n.obtainOperator(ctx)
	if operator == nil {
		return "", fmt.Errorf("no netbridge operator available")
	}

	if upnpOperator, ok := operator.(interface {
		PublicIP(context.Context) (string, error)
	}); ok {
		return upnpOperator.PublicIP(ctx)
	}

	return "", fmt.Errorf("operator does not support public IP lookup")
}

func (n *NetbridgeImpl) LocalIP(
	ctx context.Context,
) (string, error) {
	if !n.IsEnabled() {
		return "", fmt.Errorf("netbridge disabled")
	}

	operator := n.obtainOperator(ctx)
	if operator == nil {
		return "", fmt.Errorf("no netbridge operator available")
	}

	if upnpOperator, ok := operator.(interface {
		LocalIP(context.Context) (string, error)
	}); ok {
		return upnpOperator.LocalIP(ctx)
	}

	return "", fmt.Errorf("operator does not support local IP lookup")
}

func (n *NetbridgeImpl) IsPortAvailable(
	ctx context.Context,
	port int,
	protocol domain.Protocol,
) (bool, error) {
	if !n.IsEnabled() {
		return false, nil
	}

	if !n.isPortAllowed(port) {
		return false, nil
	}

	operator := n.obtainOperator(ctx)
	if operator == nil {
		return false, nil
	}

	return operator.IsPortAvailable(ctx, port, protocol)
}

func (n *NetbridgeImpl) IsProtocolAvailable(
	ctx context.Context,
	protocol domain.Protocol,
) (bool, error) {
	if !n.IsEnabled() {
		return false, nil
	}

	operator := n.obtainOperator(ctx)
	if operator == nil {
		return false, nil
	}

	return operator.IsProtocolAvailable(ctx, protocol)
}

func (n *NetbridgeImpl) ForwardRule(
	ctx context.Context,
	rule domain.Rule,
) (domain.Port, error) {
	if !n.IsEnabled() {
		return domain.Port{}, fmt.Errorf("netbridge disabled")
	}

	operator := n.obtainOperator(ctx)
	if operator == nil {
		return domain.Port{}, fmt.Errorf("no netbridge operator available")
	}

	port, err := operator.ForwardRule(ctx, rule)
	if err != nil {
		return domain.Port{}, err
	}

	if !n.isPortAllowed(port.StartPort) || !n.isPortAllowed(port.EndPort) {
		return domain.Port{}, fmt.Errorf("port %d-%d not allowed by netbridge configuration",
			port.StartPort, port.EndPort)
	}

	return port, nil
}

func (n *NetbridgeImpl) ForwardPort(
	ctx context.Context,
	port domain.Port,
) (domain.Port, error) {
	if !n.IsEnabled() {
		return domain.Port{}, fmt.Errorf("netbridge disabled")
	}

	if !n.isPortAllowed(port.StartPort) || !n.isPortAllowed(port.EndPort) {
		return domain.Port{}, fmt.Errorf("port %d-%d not allowed by netbridge configuration",
			port.StartPort, port.EndPort)
	}

	operator := n.obtainOperator(ctx)
	if operator == nil {
		return domain.Port{}, fmt.Errorf("no netbridge operator available")
	}

	return operator.ForwardPort(ctx, port)
}

func (n *NetbridgeImpl) ReversePort(
	ctx context.Context,
	port domain.Port,
) (domain.Port, error) {
	if !n.IsEnabled() {
		return domain.Port{}, fmt.Errorf("netbridge disabled")
	}

	if !n.isPortAllowed(port.StartPort) || !n.isPortAllowed(port.EndPort) {
		return domain.Port{}, fmt.Errorf("port %d-%d not allowed by netbridge configuration",
			port.StartPort, port.EndPort)
	}

	operator := n.obtainOperator(ctx)
	if operator == nil {
		return domain.Port{}, fmt.Errorf("no netbridge operator available")
	}

	return operator.ReversePort(ctx, port)
}

func (n *NetbridgeImpl) GetPortForwardingStatus(
	ctx context.Context,
	port domain.Port,
) (domain.ForwardingStatus, error) {
	if !n.IsEnabled() {
		return domain.ForwardingStatusError, fmt.Errorf("netbridge disabled")
	}

	if !n.isPortAllowed(port.StartPort) || !n.isPortAllowed(port.EndPort) {
		return domain.ForwardingStatusError, fmt.Errorf("port %d-%d not allowed by netbridge configuration",
			port.StartPort, port.EndPort)
	}

	operator := n.obtainOperator(ctx)
	if operator == nil {
		return domain.ForwardingStatusError, fmt.Errorf("no netbridge operator available")
	}

	return operator.GetPortForwardingStatus(ctx, port)
}

func (n *NetbridgeImpl) obtainOperator(
	ctx context.Context,
) interfaces.OperatorInterface {
	if n.operator != nil {
		return n.operator
	}

	if n.operators == nil {
		n.operators = operators.NewOperatorContainer()
	}

	n.operator = n.operators.Obtain(ctx)
	return n.operator
}

func (n *NetbridgeImpl) isPortAllowed(port int) bool {
	minPort, maxPort, hasRange, err := parseAllowedPorts(config.GetNetbridge().AllowedPorts)
	if err != nil {
		return false
	}

	if !hasRange {
		return true
	}

	return port >= minPort && port <= maxPort
}

func parseAllowedPorts(allowed string) (int, int, bool, error) {
	allowed = strings.TrimSpace(allowed)
	if allowed == "" {
		return 0, 0, false, nil
	}

	rangeParts := strings.Split(allowed, "-")
	if len(rangeParts) == 1 {
		port, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
		if err != nil {
			return 0, 0, false, fmt.Errorf("invalid allowed_ports value: %w", err)
		}
		return port, port, true, nil
	}

	if len(rangeParts) != 2 {
		return 0, 0, false, fmt.Errorf("invalid allowed_ports range: %s", allowed)
	}

	minPort, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
	if err != nil {
		return 0, 0, false, fmt.Errorf("invalid allowed_ports start: %w", err)
	}

	maxPort, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
	if err != nil {
		return 0, 0, false, fmt.Errorf("invalid allowed_ports end: %w", err)
	}

	if minPort <= 0 || maxPort <= 0 || minPort > maxPort {
		return 0, 0, false, fmt.Errorf("invalid allowed_ports range: %s", allowed)
	}

	return minPort, maxPort, true, nil
}
