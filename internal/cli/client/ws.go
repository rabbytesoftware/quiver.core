package client

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

// SubscribeRuntime opens the runtime WebSocket channel for a namespace (or
// glob) and streams pushed snapshots. The returned channel closes when the
// context is cancelled or the server closes the connection. Subscribe before
// firing the method POST so no step events are missed.
func (c *Client) SubscribeRuntime(
	ctx context.Context,
	ns string,
) (<-chan apidto.ArrowRuntimeDTO, error) {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	if c.socket != "" {
		socket := c.socket
		dialer.NetDialContext = func(dctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(dctx, "unix", socket)
		}
	}

	url := c.wsURL + "/v0/runtime/" + encodeNS(ns)
	conn, err := c.dialRuntime(ctx, dialer, url)
	if err != nil {
		return nil, err
	}

	events := make(chan apidto.ArrowRuntimeDTO)
	go pumpRuntime(ctx, conn, events)
	return events, nil
}

// dialRuntime dials url, attaching an Authorization: Bearer header when a
// token is set. On a 401 handshake response it retries once through
// onUnauthorized for a fresh token, mirroring roundtrip's contract for HTTP
// requests — see UnauthorizedHandler's doc comment.
func (c *Client) dialRuntime(
	ctx context.Context,
	dialer websocket.Dialer,
	url string,
) (*websocket.Conn, error) {
	conn, resp, err := dialer.DialContext(ctx, url, authHeader(c.token))
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		return conn, nil
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized || c.onUnauthorized == nil {
		return nil, &ConnError{Server: c.baseURL, Err: err}
	}

	token, pairErr := c.onUnauthorized(ctx, c)
	if pairErr != nil {
		return nil, &ConnError{Server: c.baseURL, Err: err}
	}
	c.token = token

	conn, resp, err = dialer.DialContext(ctx, url, authHeader(c.token))
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, &ConnError{Server: c.baseURL, Err: err}
	}
	return conn, nil
}

// authHeader builds the WebSocket handshake header carrying token, or nil
// when there is nothing to send.
func authHeader(token string) http.Header {
	if token == "" {
		return nil
	}
	return http.Header{"Authorization": []string{"Bearer " + token}}
}

// pumpRuntime reads frames until the connection dies or ctx is cancelled.
func pumpRuntime(
	ctx context.Context,
	conn *websocket.Conn,
	events chan<- apidto.ArrowRuntimeDTO,
) {
	defer close(events)
	defer func() { _ = conn.Close() }()

	// Close the connection when ctx is cancelled so ReadMessage unblocks.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	for {
		_, frame, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var dto apidto.ArrowRuntimeDTO
		if err := json.Unmarshal(frame, &dto); err != nil {
			continue
		}
		select {
		case events <- dto:
		case <-ctx.Done():
			return
		}
	}
}
