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
	"sync"
	"time"
)

// Client talks to one quiver.core instance.
type Client struct {
	http    *http.Client
	baseURL string
	wsURL   string
	socket  string

	// tokenMu guards token: send (and dialRuntime, in ws.go) read it to
	// build every outbound request's Authorization header, while roundtrip
	// (and dialRuntime) write it after a 401 recovery — both can run
	// concurrently when a caller fires off parallel requests.
	tokenMu        sync.RWMutex
	token          string
	onUnauthorized UnauthorizedHandler
}

// getToken returns the current bearer token, safe for concurrent use.
func (c *Client) getToken() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.token
}

// setToken updates the bearer token, safe for concurrent use.
func (c *Client) setToken(token string) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.token = token
}

// UnauthorizedHandler is called once when a request comes back 401, to
// obtain a fresh token to retry with. A nil handler (the default) means a
// 401 is returned to the caller unretried — this is how a unix:// client
// behaves, since the daemon never gates that scheme.
type UnauthorizedHandler func(ctx context.Context, c *Client) (token string, err error)

// Option configures a Client at construction time.
type Option func(*Client)

// WithToken attaches an Authorization: Bearer header to every request.
func WithToken(token string) Option {
	return func(c *Client) { c.token = token }
}

// WithUnauthorizedHandler registers fn to run on a 401 response. Its
// returned token is used for one retry of the same request; a returned
// error surfaces the original 401 to the caller instead.
func WithUnauthorizedHandler(fn UnauthorizedHandler) Option {
	return func(c *Client) { c.onUnauthorized = fn }
}

// New builds a client for a server URI. Accepted forms:
// "unix:///path/to/quiver.sock", "tcp://host:port", "http://host:port",
// "https://host:port".
func New(server string, opts ...Option) (*Client, error) {
	if server == "" {
		return nil, fmt.Errorf("client: server URI is empty")
	}

	u, err := url.Parse(server)
	if err != nil {
		return nil, fmt.Errorf("client: parse server URI %q: %w", server, err)
	}

	var c *Client
	switch u.Scheme {
	case "unix":
		c = newUnixClient(u)
	case "tcp":
		c = newTCPClient("http://" + u.Host)
	case "http", "https":
		c = newTCPClient(server)
	default:
		return nil, fmt.Errorf("client: unsupported scheme %q in %q", u.Scheme, server)
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
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

// send performs one request attempt and returns the raw status and body.
// reqBody may be nil.
func (c *Client) send(
	ctx context.Context,
	method, path string,
	reqBody []byte,
) (int, []byte, error) {
	var body io.Reader
	if reqBody != nil {
		body = bytes.NewReader(reqBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return 0, nil, fmt.Errorf("client: build request %s %s: %w", method, path, err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token := c.getToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, &ConnError{Server: c.baseURL, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("client: read response %s %s: %w", method, path, err)
	}
	return resp.StatusCode, raw, nil
}

// roundtrip performs a request and returns the raw status and body,
// transparently retrying once through onUnauthorized on a 401 — see
// UnauthorizedHandler's doc comment.
func (c *Client) roundtrip(
	ctx context.Context,
	method, path string,
	reqBody []byte,
) (int, []byte, error) {
	status, raw, err := c.send(ctx, method, path, reqBody)
	if err != nil {
		return 0, nil, err
	}
	if status != http.StatusUnauthorized || c.onUnauthorized == nil {
		return status, raw, nil
	}

	token, pairErr := c.onUnauthorized(ctx, c)
	if pairErr != nil {
		// The original 401 is no longer the useful signal here: the caller
		// asked to auto-recover and recovery itself failed, so this is a
		// connectivity/auth-repair failure, not "the daemon rejected this
		// specific request" — matching dialRuntime's WS-side contract for
		// the same failure shape.
		return 0, nil, &ConnError{Server: c.baseURL, Err: pairErr}
	}
	c.setToken(token)

	return c.send(ctx, method, path, reqBody)
}

// do performs a request and returns the decoded envelope data. A nil out
// skips data decoding. reqBody may be nil.
func (c *Client) do(
	ctx context.Context,
	method, path string,
	reqBody []byte,
	out any,
) error {
	status, raw, err := c.roundtrip(ctx, method, path, reqBody)
	if err != nil {
		return err
	}
	return c.decode(status, raw, out)
}

// doMutation performs a mutating request and reports whether the server
// accepted asynchronous work (202) rather than completing it as a no-op (200).
func (c *Client) doMutation(
	ctx context.Context,
	method, path string,
	reqBody []byte,
) (bool, error) {
	status, raw, err := c.roundtrip(ctx, method, path, reqBody)
	if err != nil {
		return false, err
	}
	if err := c.decode(status, raw, nil); err != nil {
		return false, err
	}
	return status == http.StatusAccepted, nil
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
