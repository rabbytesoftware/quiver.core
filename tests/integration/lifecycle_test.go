//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// waitForListLen polls the List endpoint until the response contains wantLen items
// or the timeout is exceeded. The list projection is updated asynchronously, so a
// brief wait is needed after Add before the store reflects the change.
func waitForListLen(
	t *testing.T,
	c *client,
	wantLen int,
	timeout time.Duration,
) []any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := c.List()
		if resp.StatusCode == http.StatusOK {
			var outer map[string]any
			decodeJSON(t, resp, &outer)
			list, _ := outer["data"].([]any)
			if len(list) == wantLen {
				return list
			}
		} else {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("waitForListLen: timeout waiting for %d items in list", wantLen)
	return nil
}

func waitForState(
	t *testing.T,
	c *client,
	ns string,
	want domain.ArrowState,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last domain.ArrowState
	for time.Now().Before(deadline) {
		resp := c.GetDetail(ns)
		if resp.StatusCode == http.StatusOK {
			var outer map[string]any
			decodeJSON(t, resp, &outer)
			if data, ok := outer["data"].(map[string]any); ok {
				if s, ok := data["state"].(string); ok {
					last = domain.ArrowState(s)
					if last == want {
						return
					}
				}
			}
		} else {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("waitForState(%s): timeout waiting for %s, last=%s", ns, want, last)
}

func (s *IntegrationSuite) TestLifecycle_FullRoundTrip() {
	env := s.newEnv()
	c := env.client(s.T())

	// Add
	resp := c.Add(nsFor("quiver-test/tool-a", "v1"))
	mustStatus(s.T(), resp, http.StatusCreated)

	// Appears in list — projection is async, poll until ready
	waitForListLen(s.T(), c, 1, 5*time.Second)

	// Install
	resp = c.Install(nsFor("quiver-test/tool-a", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 15*time.Second)

	// Execute
	resp = c.Execute(nsFor("quiver-test/tool-a", "v1"), "execute", nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 15*time.Second)

	// Uninstall
	resp = c.Uninstall(nsFor("quiver-test/tool-a", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, 15*time.Second)

	// Remove
	resp = c.Remove(nsFor("quiver-test/tool-a", "v1"))
	mustStatus(s.T(), resp, http.StatusOK)

	// Gone from API
	resp = c.GetDetail(nsFor("quiver-test/tool-a", "v1"))
	mustStatus(s.T(), resp, http.StatusNotFound)
}

func (s *IntegrationSuite) TestLifecycle_AddIdempotency() {
	env := s.newEnv()
	c := env.client(s.T())
	ns := nsFor("quiver-test/tool-a", "v1")

	// First add: created
	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	// Second add: catalog is idempotent — returns 201 again, no conflict
	resp = c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	// Arrow still appears exactly once in the list — projection is async
	list := waitForListLen(s.T(), c, 1, 5*time.Second)
	s.Len(list, 1)
}

func (s *IntegrationSuite) TestLifecycle_StateViaWebSocket() {
	env := s.newEnv()
	c := env.client(s.T())
	ns := nsFor("quiver-test/tool-a", "v1")

	// Add arrow first
	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	// Connect WebSocket BEFORE installing
	conn, err := c.DialRuntime(ns)
	s.Require().NoError(err)
	defer conn.Close()

	// Set read deadline so we don't hang
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))

	// Start install concurrently
	installDone := make(chan struct{})
	go func() {
		defer close(installDone)
		resp := c.Install(ns, nil)
		resp.Body.Close()
	}()

	// Collect state messages
	var states []string
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break // deadline or close
		}
		var payload map[string]any
		if json.Unmarshal(msg, &payload) == nil {
			if state, ok := payload["state"].(string); ok {
				states = append(states, state)
				if state == string(domain.ArrowStateReady) {
					break
				}
			}
		}
	}

	<-installDone

	// Check that installing appeared before ready
	installIdx := -1
	readyIdx := -1
	for i, st := range states {
		if st == string(domain.ArrowStateInstalling) {
			installIdx = i
		}
		if st == string(domain.ArrowStateReady) {
			readyIdx = i
		}
	}
	s.Greater(readyIdx, installIdx, "ready should come after installing, states: %v", states)
	s.GreaterOrEqual(installIdx, 0, "installing state should have appeared")
}

func (s *IntegrationSuite) TestLifecycle_ExecuteUnknownMethod() {
	env := s.newEnv()
	c := env.client(s.T())
	ns := nsFor("quiver-test/tool-a", "v1")

	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Install(ns, nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, ns, domain.ArrowStateReady, 15*time.Second)

	// Execute unknown method — falls through to default BeginExecution path
	// which returns ErrMethodNotFound → 404
	resp = c.Execute(ns, "_unknownxyz", nil)
	s.GreaterOrEqual(resp.StatusCode, 400)
	s.Less(resp.StatusCode, 600)
	resp.Body.Close()

	// State unchanged — still ready
	waitForState(s.T(), c, ns, domain.ArrowStateReady, 5*time.Second)
}
