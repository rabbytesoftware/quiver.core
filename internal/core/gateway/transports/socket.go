package transports

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
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

	// *net.UnixListener unlinks the path itself on Close, unconditionally --
	// the same unsafe-on-a-race behaviour socketListener.Close exists to
	// replace, not add to. Left enabled, it deletes a sibling's live socket
	// before our own, ownership-checked removal ever runs. Unix-only: this
	// type assertion fails harmlessly on Windows, which has no such default
	// to disable.
	if unixLn, ok := ln.(*net.UnixListener); ok {
		unixLn.SetUnlinkOnClose(false)
	}

	// Both remaining failure paths below abort before this function ever
	// hands the listener back, so nothing else can have raced onto path yet
	// -- unlike socketListener.Close, an unconditional removal here is safe,
	// and is now this function's own job now that SetUnlinkOnClose(false)
	// means ln.Close() no longer does it.
	if runtime.GOOS != "windows" {
		if err := chmod(s.path, 0o600); err != nil {
			_ = ln.Close()
			_ = os.Remove(s.path)
			return nil, fmt.Errorf("gateway: socket: chmod %s: %w", s.path, err)
		}
	}

	// Recorded now, while this call still knows the file at path is the one
	// net.Listen just created -- see socketListener.Close for why Close needs
	// this rather than trusting the path alone.
	boundInfo, err := os.Stat(s.path)
	if err != nil {
		_ = ln.Close()
		_ = os.Remove(s.path)
		return nil, fmt.Errorf("gateway: socket: stat %s: %w", s.path, err)
	}

	return &socketListener{Listener: ln, path: s.path, boundInfo: boundInfo}, nil
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
