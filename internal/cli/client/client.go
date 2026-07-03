// Package client is the CLI's HTTP + WebSocket client for the quiver.core
// API. It speaks the v0 envelope over a Unix domain socket (local daemon) or
// TCP (remote contexts) and maps API failures to CLI exit codes.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to one quiver.core instance.
type Client struct {
	http    *http.Client
	baseURL string
	wsURL   string
	socket  string
}

// New builds a client for a server URI. Accepted forms:
// "unix:///path/to/quiver.sock", "tcp://host:port", "http://host:port",
// "https://host:port".
func New(server string) (*Client, error) {
	if server == "" {
		return nil, fmt.Errorf("client: server URI is empty")
	}

	u, err := url.Parse(server)
	if err != nil {
		return nil, fmt.Errorf("client: parse server URI %q: %w", server, err)
	}

	switch u.Scheme {
	case "unix":
		return newUnixClient(u), nil
	case "tcp":
		return newTCPClient("http://" + u.Host), nil
	case "http", "https":
		return newTCPClient(server), nil
	default:
		return nil, fmt.Errorf("client: unsupported scheme %q in %q", u.Scheme, server)
	}
}

func newUnixClient(u *url.URL) *Client {
	socket := u.Path
	if u.Host != "" {
		socket = "/" + u.Host + u.Path
	}
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", socket)
	}
	return &Client{
		http: &http.Client{
			Transport: &http.Transport{DialContext: dial},
			Timeout:   30 * time.Second,
		},
		// The host segment is a placeholder — the dialer ignores it.
		baseURL: "http://quiver",
		wsURL:   "ws://quiver",
		socket:  socket,
	}
}

func newTCPClient(base string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: strings.TrimSuffix(base, "/"),
		wsURL:   "ws" + strings.TrimPrefix(strings.TrimSuffix(base, "/"), "http"),
	}
}

// encodeNS percent-encodes a namespace into a single path segment.
func encodeNS(ns string) string {
	return url.PathEscape(ns)
}

// envelope is the shared v0 response wrapper.
type envelope struct {
	Success   bool            `json:"success"`
	Error     string          `json:"error"`
	Namespace string          `json:"namespace"`
	Data      json.RawMessage `json:"data"`
}

// do performs a request and returns the decoded envelope data. A nil out
// skips data decoding. reqBody may be nil.
func (c *Client) do(
	ctx context.Context,
	method, path string,
	reqBody []byte,
	out any,
) error {
	var body io.Reader
	if reqBody != nil {
		body = bytes.NewReader(reqBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("client: build request %s %s: %w", method, path, err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return &ConnError{Server: c.baseURL, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("client: read response %s %s: %w", method, path, err)
	}

	return c.decode(resp.StatusCode, raw, out)
}

// decode unwraps the envelope, mapping failures to APIError.
func (c *Client) decode(status int, raw []byte, out any) error {
	var env envelope
	envErr := json.Unmarshal(raw, &env)

	if status >= http.StatusBadRequest {
		msg := strings.TrimSpace(string(raw))
		if envErr == nil && env.Error != "" {
			msg = env.Error
		}
		return &APIError{Status: status, Message: msg, Namespace: env.Namespace}
	}

	if envErr == nil && !env.Success && env.Error != "" {
		return &APIError{Status: status, Message: env.Error, Namespace: env.Namespace}
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("client: decode response data: %w", err)
	}
	return nil
}

// doRaw performs a request whose response is not enveloped (e.g. /v0/health).
func (c *Client) doRaw(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("client: build request GET %s: %w", path, err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return &ConnError{Server: c.baseURL, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("client: read response GET %s: %w", path, err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("client: decode response: %w", err)
	}
	return nil
}
