package domain

import "fmt"

const (
	MinPort = 1
	MaxPort = 65535
)

type PortRule struct {
	ID               string           `json:"id"`
	StartPort        int              `json:"start_port"`
	EndPort          int              `json:"end_port"`
	Protocol         Protocol         `json:"protocol"`
	ForwardingStatus ForwardingStatus `json:"forwarding_status"`
}

func (p *PortRule) IsStartPortValid() bool {
	return p.StartPort > 0 && p.StartPort <= 65535
}

func (p *PortRule) IsEndPortValid() bool {
	return p.EndPort > 0 && p.EndPort <= 65535
}

func (p *PortRule) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("port rule ID cannot be empty")
	}

	if p.StartPort > 0 && (p.StartPort < MinPort || p.StartPort > MaxPort) {
		return fmt.Errorf("start_port must be between %d and %d, got %d", MinPort, MaxPort, p.StartPort)
	}

	if p.EndPort > 0 && (p.EndPort < MinPort || p.EndPort > MaxPort) {
		return fmt.Errorf("end_port must be between %d and %d, got %d", MinPort, MaxPort, p.EndPort)
	}

	if p.StartPort > 0 && p.EndPort > 0 && p.StartPort > p.EndPort {
		return fmt.Errorf("start_port (%d) cannot be greater than end_port (%d)", p.StartPort, p.EndPort)
	}

	if !p.Protocol.IsValid() {
		return fmt.Errorf("invalid protocol: %s", p.Protocol)
	}

	return nil
}
