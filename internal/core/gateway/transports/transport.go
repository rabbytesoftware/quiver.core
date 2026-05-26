package transports

import "net"

// Transport creates a network listener for the daemon.
type Transport interface {
	Listen() (net.Listener, error)
}
