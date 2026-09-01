package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway"
)

// daemonClientTimeout bounds every request the CLI makes to the daemon. The
// admin endpoints it calls are all local, in-process reads and writes —
// nothing here waits on the network.
const daemonClientTimeout = 10 * time.Second

// daemonClient is a minimal HTTP client for a running quiver daemon. It is
// deliberately separate from the daemon's own DI wiring (internal.New): the
// CLI is a short-lived process talking to a long-lived one over the network
// or a Unix socket, never in-process, so it has no engines, adapters, or
// usecases of its own to construct.
type daemonClient struct {
	http    *http.Client
	baseURL string
}

// newDaemonClient resolves the daemon's host URI — hostOverride if set,
// otherwise the same api.host the daemon itself reads from config — and
// returns a client dialed to it.
func newDaemonClient(
	hostOverride string,
) (*daemonClient, error) {
	host := hostOverride
	if host == "" {
		host = config.GetAPI().Host
	}

	scheme, authority, err := gateway.Scheme(host)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	switch scheme {
	case "unix":
		return newUnixDaemonClient(gateway.SocketPath(authority)), nil
	case "tcp":
		return &daemonClient{
			http:    &http.Client{Timeout: daemonClientTimeout},
			baseURL: "http://" + authority,
		}, nil
	default:
		return nil, fmt.Errorf("auth: unsupported scheme %q in host URI %q", scheme, host)
	}
}

func newUnixDaemonClient(
	socketPath string,
) *daemonClient {
	return &daemonClient{
		http: &http.Client{
			Timeout: daemonClientTimeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
		// The host in this URL is never resolved — DialContext ignores it and
		// always dials the socket above — it exists only because net/http
		// requires a well-formed URL.
		baseURL: "http://unix",
	}
}

// do sends body (nil for none) as JSON and decodes the response into out.
func (c *daemonClient) do(
	ctx context.Context,
	method string,
	path string,
	body any,
	out any,
) error {
	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("auth: encode request: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("auth: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("auth: request daemon: %w", err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("auth: decode response: %w", err)
	}

	return nil
}
