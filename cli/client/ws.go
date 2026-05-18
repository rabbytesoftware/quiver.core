package client

import (
	"context"
	"encoding/json"

	"github.com/gorilla/websocket"
)

// pump dials wsURL and delivers ArrowRuntime snapshots to the returned channel.
// The channel is closed when stopFn returns true or ctx is cancelled.
// Closing the WS connection (on ctx cancel) unblocks ReadMessage — the gorilla pattern
// for context-aware WS reads.
func pump(ctx context.Context, wsURL string, stopFn func(rt ArrowRuntime, sawRun bool) bool) (<-chan ArrowRuntime, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}

	ch := make(chan ArrowRuntime, 16)
	done := make(chan struct{})

	go func() {
		defer conn.Close()
		defer close(ch)
		defer close(done)

		go func() {
			select {
			case <-ctx.Done():
				conn.Close()
			case <-done:
				// pump exited naturally; watcher exits cleanly
			}
		}()

		sawRun := false
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var rt ArrowRuntime
			if err := json.Unmarshal(msg, &rt); err != nil {
				// silently skip malformed frames
				continue
			}

			if rt.ActiveRun != nil {
				sawRun = true
			}

			select {
			case ch <- rt:
			case <-ctx.Done():
				return
			}

			if stopFn(rt, sawRun) {
				return
			}
		}
	}()

	return ch, nil
}

func terminalInstall(rt ArrowRuntime, _ bool) bool {
	return rt.State == "ready" || rt.State == "absent"
}

func terminalUninstall(rt ArrowRuntime, _ bool) bool {
	// "removed" is the normal success state; "ready" covers the case where the
	// arrow was never installed (server treats it as a no-op, returns "ready").
	return rt.State == "removed" || rt.State == "ready"
}

func terminalReady(rt ArrowRuntime, _ bool) bool {
	return rt.State == "ready"
}

// terminalCustomMethod closes after ActiveRun goes nil following a non-nil snapshot.
func terminalCustomMethod(rt ArrowRuntime, sawRun bool) bool {
	return sawRun && rt.ActiveRun == nil
}

func neverStop(_ ArrowRuntime, _ bool) bool { return false }
