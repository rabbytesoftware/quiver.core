package client

import (
	"context"
	"encoding/json"
	"net"
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
	conn, resp, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, &ConnError{Server: c.baseURL, Err: err}
	}

	events := make(chan apidto.ArrowRuntimeDTO)
	go pumpRuntime(ctx, conn, events)
	return events, nil
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
