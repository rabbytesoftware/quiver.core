package domain

type Protocol string

const (
	ProtocolTCP    Protocol = "tcp"
	ProtocolUDP    Protocol = "udp"
	ProtocolTCPUDP Protocol = "tcp/udp"
)

func (p Protocol) String() string {
	return string(p)
}

func (p Protocol) IsValid() bool {
	return p == ProtocolTCP || p == ProtocolUDP || p == ProtocolTCPUDP
}

func (p Protocol) IsTCP() bool {
	return p == ProtocolTCP
}

func (p Protocol) IsUDP() bool {
	return p == ProtocolUDP
}

func (p Protocol) IsTCPUDP() bool {
	return p == ProtocolTCPUDP
}
