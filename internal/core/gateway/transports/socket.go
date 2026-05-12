package transports

import (
	"errors"
	"fmt"
	"net"
	"os"
)

var chmod = os.Chmod

// ErrDaemonRunning is returned when a Quiver daemon is already listening on the socket path.
var ErrDaemonRunning = errors.New("gateway: socket: daemon already running")

type socketTransport struct {
	path string
}

// NewSocket returns a Transport that binds a Unix domain socket at path.
func NewSocket(
	path string,
) Transport {
	return &socketTransport{path: path}
}

func (s *socketTransport) Listen() (net.Listener, error) {
	if err := s.handleStale(); err != nil {
		return nil, err
	}

	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return nil, fmt.Errorf("gateway: socket: listen %s: %w", s.path, err)
	}

	if err := chmod(s.path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("gateway: socket: chmod %s: %w", s.path, err)
	}

	return &socketListener{Listener: ln, path: s.path}, nil
}

func (s *socketTransport) handleStale() error {
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		return nil
	}

	conn, err := net.Dial("unix", s.path)
	if err != nil {
		return os.Remove(s.path)
	}

	_ = conn.Close()
	return ErrDaemonRunning
}
