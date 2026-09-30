package transports

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// ErrPipeUnsupported is returned by the named-pipe transport on platforms
// other than Windows.
var ErrPipeUnsupported = errors.New("gateway: npipe: named pipes are only supported on windows")

type npipeTransport struct {
	path string
}

// NewNPipe returns a Transport that binds a Windows named pipe at path
// (e.g. \\.\pipe\quiver), reachable only by the current user.
func NewNPipe(
	path string,
) Transport {
	return &npipeTransport{path: path}
}

func (t *npipeTransport) Listen() (net.Listener, error) {
	ln, err := listenPipe(t.path)
	if err != nil {
		return nil, fmt.Errorf("gateway: npipe: listen %s: %w", t.path, err)
	}
	return ln, nil
}

// DialPipe connects to the named pipe at path.
func DialPipe(
	ctx context.Context,
	path string,
) (net.Conn, error) {
	return dialPipe(ctx, path)
}
