package gateway

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway/transports"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
)

const (
	pipePrefix = `\\.\pipe\`
	pipeName   = "quiver"
)

// New creates a network listener from the API config host URI.
// Supports unix:// (Unix domain socket), npipe:// (Windows named pipe) and
// tcp:// (TCP) schemes. For unix://, an empty path resolves to the Quiver
// home socket, except on Windows where it resolves to the named pipe.
func New(
	cfg config.API,
) (net.Listener, error) {
	scheme, authority, err := Scheme(cfg.Host)
	if err != nil {
		return nil, err
	}

	if scheme == "unix" && authority == "" && runtime.GOOS == "windows" {
		scheme = "npipe"
	}

	switch scheme {
	case "unix":
		return transports.NewSocket(SocketPath(authority)).Listen()
	case "npipe":
		return transports.NewNPipe(PipePath(authority)).Listen()
	case "tcp":
		return transports.NewTCP(authority).Listen()
	default:
		return nil, fmt.Errorf("gateway: unsupported scheme %q in host URI %q", scheme, cfg.Host)
	}
}

// Scheme splits a host URI into its scheme ("unix", "npipe" or "tcp") and authority.
// Exported so callers that need to know the scheme without opening a
// listener — internal.Container.Start decides whether device-pairing auth
// applies from this alone — don't duplicate the parsing New does.
func Scheme(
	hostURI string,
) (scheme, authority string, err error) {
	const sep = "://"
	idx := strings.Index(hostURI, sep)
	if idx < 0 {
		return "", "", fmt.Errorf("gateway: invalid host URI %q: missing ://", hostURI)
	}

	return hostURI[:idx], hostURI[idx+len(sep):], nil
}

// SocketPath resolves the Unix socket path for a unix:// host URI's
// authority. An empty override resolves to the Quiver home socket. Exported
// so the CLI's daemon client (cmd/quiver) can dial the same socket the
// daemon listens on, without duplicating this resolution.
func SocketPath(
	override string,
) string {
	if override != "" {
		return override
	}
	return filepath.Join(metadata.GetHomePath(), "quiver.sock")
}

// PipePath resolves the Windows named-pipe path for an npipe:// host URI's
// authority. An empty name resolves to the Quiver pipe.
func PipePath(
	name string,
) string {
	if name == "" {
		name = pipeName
	}
	return pipePrefix + name
}

// LocalSocket returns the default local daemon endpoint: the named pipe on
// Windows, otherwise the socket under dir.
func LocalSocket(
	dir string,
) string {
	if runtime.GOOS == "windows" {
		return PipePath("")
	}
	return filepath.Join(dir, "quiver.sock")
}

// LocalURI turns a local endpoint as LocalSocket returns it into the host
// URI a client dials.
func LocalURI(
	socket string,
) string {
	if name, ok := strings.CutPrefix(socket, pipePrefix); ok {
		return "npipe://" + name
	}
	return "unix://" + socket
}

// DialURI turns the local address of a connection a listener accepted into the
// host URI a client dials to reach that same listener; empty for a nil address.
func DialURI(
	addr net.Addr,
) string {
	if addr == nil {
		return ""
	}
	if tcp, ok := addr.(*net.TCPAddr); ok {
		return "tcp://" + tcp.String()
	}
	return LocalURI(addr.String())
}

// Dial connects to a local daemon endpoint as LocalSocket returns it.
func Dial(
	ctx context.Context,
	socket string,
) (net.Conn, error) {
	if strings.HasPrefix(socket, pipePrefix) {
		return transports.DialPipe(ctx, socket)
	}

	var d net.Dialer
	return d.DialContext(ctx, "unix", socket)
}
