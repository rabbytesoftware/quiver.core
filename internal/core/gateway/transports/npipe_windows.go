//go:build windows

package transports

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func listenPipe(
	path string,
) (net.Listener, error) {
	sddl, err := currentUserSDDL()
	if err != nil {
		return nil, err
	}

	// go-winio creates the first instance with FILE_CREATE, so a live
	// daemon's pipe makes this fail instead of silently sharing the name.
	ln, err := winio.ListenPipe(path, &winio.PipeConfig{SecurityDescriptor: sddl})
	if err != nil {
		if isPipeTaken(err) {
			return nil, fmt.Errorf("%w: %w", ErrDaemonRunning, err)
		}
		return nil, err
	}
	return ln, nil
}

func dialPipe(
	ctx context.Context,
	path string,
) (net.Conn, error) {
	return winio.DialPipeContext(ctx, path)
}

// currentUserSDDL builds a protected DACL granting full access to the
// current user alone, the named-pipe counterpart of the unix socket's 0600.
func currentUserSDDL() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("gateway: npipe: resolve current user: %w", err)
	}
	return "D:P(A;;GA;;;" + user.User.Sid.String() + ")", nil
}

func isPipeTaken(
	err error,
) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_ALREADY_EXISTS) ||
		errors.Is(err, windows.ERROR_PIPE_BUSY)
}
