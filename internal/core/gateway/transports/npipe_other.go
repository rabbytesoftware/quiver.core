//go:build !windows

package transports

import (
	"context"
	"net"
)

func listenPipe(
	_ string,
) (net.Listener, error) {
	return nil, ErrPipeUnsupported
}

func dialPipe(
	_ context.Context,
	_ string,
) (net.Conn, error) {
	return nil, ErrPipeUnsupported
}
