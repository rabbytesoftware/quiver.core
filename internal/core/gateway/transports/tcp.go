package transports

import (
	"fmt"
	"net"
)

type tcpTransport struct {
	addr string
}

// NewTCP returns a Transport that binds a TCP listener on addr.
func NewTCP(
	addr string,
) Transport {
	return &tcpTransport{addr: addr}
}

func (t *tcpTransport) Listen() (net.Listener, error) {
	ln, err := net.Listen("tcp", t.addr)
	if err != nil {
		return nil, fmt.Errorf("gateway: tcp: listen %s: %w", t.addr, err)
	}
	return ln, nil
}
