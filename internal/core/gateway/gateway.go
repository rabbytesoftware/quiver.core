package gateway

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"

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
	u, err := url.Parse(cfg.Host)
	if err != nil {
		return nil, fmt.Errorf("gateway: invalid host URI %q: %w", cfg.Host, err)
	}

	switch u.Scheme {
	case "unix":
		return transports.NewSocket(socketPath(u.Path)).Listen()
	case "tcp":
		return transports.NewTCP(u.Host).Listen()
	default:
		return nil, fmt.Errorf("gateway: unsupported scheme %q in host URI %q", u.Scheme, cfg.Host)
	}
}

func socketPath(
	override string,
) string {
	if override != "" {
		return override
	}
	return filepath.Join(metadata.GetHomePath(), "quiver.sock")
}
