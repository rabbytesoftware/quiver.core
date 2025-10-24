package networking

import "github.com/google/uuid"

type Port struct {
	ID               uuid.UUID        `json:"id"`
	StartPort        int              `json:"start_port"`
	EndPort          int              `json:"end_port"`
	Protocol         Protocol         `json:"protocol"`
	ForwardingStatus ForwardingStatus `json:"forwarding_status"`
}

func (p *Port) IsStartPortValid() bool {
	return p.StartPort > 0 && p.StartPort <= 65535
}

func (p *Port) IsEndPortValid() bool {
	return p.EndPort > 0 && p.EndPort <= 65535
}
