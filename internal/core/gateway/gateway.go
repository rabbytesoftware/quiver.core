package gateway

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway/transports"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
)

// New creates a network listener from the API config host URI.
// Supports unix:// (Unix domain socket) and tcp:// (TCP) schemes.
// For unix://, an empty path resolves to the Quiver home socket.
func New(
	cfg config.API,
) (net.Listener, error) {
	scheme, authority, err := Scheme(cfg.Host)
	if err != nil {
		return nil, err
	}

	switch scheme {
	case "unix":
		return transports.NewSocket(socketPath(authority)).Listen()
	case "tcp":
		return transports.NewTCP(authority).Listen()
	default:
		return nil, fmt.Errorf("gateway: unsupported scheme %q in host URI %q", scheme, cfg.Host)
	}
}

// Scheme splits a host URI into its scheme ("unix" or "tcp") and authority.
// Exported so callers that need to know the scheme without opening a
// listener — internal.Container.Start decides whether device-pairing auth
// applies from this alone — don't duplicate the parsing New does.
func Scheme(
	hostURI string,
) (scheme string, authority string, err error) {
	const sep = "://"
	idx := strings.Index(hostURI, sep)
	if idx < 0 {
		return "", "", fmt.Errorf("gateway: invalid host URI %q: missing ://", hostURI)
	}

	return hostURI[:idx], hostURI[idx+len(sep):], nil
}

func socketPath(
	override string,
) string {
	if override != "" {
		return override
	}
	return filepath.Join(metadata.GetHomePath(), "quiver.sock")
}
