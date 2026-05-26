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
	const sep = "://"
	idx := strings.Index(cfg.Host, sep)
	if idx < 0 {
		return nil, fmt.Errorf("gateway: invalid host URI %q: missing ://", cfg.Host)
	}
	scheme := cfg.Host[:idx]
	authority := cfg.Host[idx+len(sep):]

	switch scheme {
	case "unix":
		return transports.NewSocket(socketPath(authority)).Listen()
	case "tcp":
		return transports.NewTCP(authority).Listen()
	default:
		return nil, fmt.Errorf("gateway: unsupported scheme %q in host URI %q", scheme, cfg.Host)
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
