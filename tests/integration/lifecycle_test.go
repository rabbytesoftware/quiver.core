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
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var outer map[string]any
		decodeJSON(t, resp, &outer)
		resp.Body.Close()
		list, _ := outer["data"].([]any)
		if len(list) == wantLen {
			return list
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
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var outer map[string]any
		decodeJSON(t, resp, &outer)
		resp.Body.Close()
		if data, ok := outer["data"].(map[string]any); ok {
			if s, ok := data["state"].(string); ok {
				last = domain.ArrowState(s)
				if last == want {
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("waitForState(%s): timeout waiting for %s, last=%s", ns, want, last)
}

func waitForInstalledRef(
	t *testing.T,
	c *client,
	timeout time.Duration,
) (ref, installedAt string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := c.List()
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			time.Sleep(50 * time.Millisecond)
			continue
		}
		var outer map[string]any
		decodeJSON(t, resp, &outer)
		resp.Body.Close()
		list, _ := outer["data"].([]any)
		if len(list) == 1 {
			item, _ := list[0].(map[string]any)
			versions, _ := item["versions"].([]any)
			if len(versions) == 1 {
				v, _ := versions[0].(map[string]any)
				ref, _ = v["ref"].(string)
				installedAt, _ = v["installed_at"].(string)
				if ref != "" {
					return ref, installedAt
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("waitForInstalledRef: timeout waiting for installed ref")
	return "", ""
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

	// Check that ready state was received (installing may be missed for fast commands)
	readyIdx := -1
	for i, st := range states {
		if st == string(domain.ArrowStateReady) {
			readyIdx = i
		}
	}
	s.GreaterOrEqual(readyIdx, 0, "ready state should have appeared in WebSocket stream, states: %v", states)
}

func (s *IntegrationSuite) TestLifecycle_ServiceStop() {
	env := s.newEnv()
	c := env.client(s.T())
	ns := nsFor("quiver-test/service-b", "v1")

	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Install(ns, nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, ns, domain.ArrowStateReady, 15*time.Second)

	// Execute starts the long-running process (sleep 300) → running state.
	resp = c.Execute(ns, "execute", nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, ns, domain.ArrowStateRunning, 15*time.Second)

	// Stop terminates the process → back to ready.
	resp = c.Stop(ns)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, ns, domain.ArrowStateReady, 15*time.Second)
}

func (s *IntegrationSuite) TestLifecycle_SeedThenInstall() {
	env := s.newEnv()
	c := env.client(s.T())
	// Seed with a ref that isn't registered in the testResolver to confirm
	// Seed itself never calls the resolver — it validates and stores raw YAML only.
	// Install still works because tool-a has no deps, so deps.Resolve never needs
	// to fetch the seeded manifest from an external source.
	ns := nsFor("quiver-test/tool-a", "v1")

	content := readFixture(s.T(), "tool-a/arrow.yaml")

	// Seed adds the arrow to catalog from a raw manifest body.
	resp := c.Seed(ns, content)
	mustStatus(s.T(), resp, http.StatusCreated)

	// Arrow must be visible in the catalog immediately after Seed.
	resp = c.GetDetail(ns)
	mustStatus(s.T(), resp, http.StatusOK)

	// Install works using the seeded manifest.
	resp = c.Install(ns, nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, ns, domain.ArrowStateReady, 15*time.Second)
}

func (s *IntegrationSuite) TestLifecycle_UpdateMethod() {
	env := s.newEnv()
	c := env.client(s.T())
	ns := nsFor("quiver-test/tool-with-update", "v1")

	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Install(ns, nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, ns, domain.ArrowStateReady, 15*time.Second)

	// Trigger the _update lifecycle method — runs the update steps and returns to ready.
	resp = c.Execute(ns, "_update", nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, ns, domain.ArrowStateReady, 15*time.Second)
}

func (s *IntegrationSuite) TestLifecycle_InstalledRefInList() {
	env := s.newEnv()
	c := env.client(s.T())
	ns := nsFor("quiver-test/tool-a", "v1")

	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Install(ns, nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, ns, domain.ArrowStateReady, 15*time.Second)

	// MarkInstalled is dispatched asynchronously after the install steps finish.
	// Poll the List endpoint until ref is populated (typically < 100ms after ready).
	ref, installedAt := waitForInstalledRef(s.T(), c, 5*time.Second)

	s.Equal("v1", ref, "installed ref in list must be v1")
	s.NotEmpty(installedAt, "installed_at must be set after install")
	s.NotEqual("0001-01-01T00:00:00Z", installedAt, "installed_at must not be zero time")
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
	mustStatus(s.T(), resp, http.StatusNotFound)

	// State unchanged — still ready
	waitForState(s.T(), c, ns, domain.ArrowStateReady, 5*time.Second)
}
