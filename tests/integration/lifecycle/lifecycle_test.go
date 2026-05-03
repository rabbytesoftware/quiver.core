//go:build integration

package lifecycle_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/tests/integration/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type LifecycleSuite struct{ kit.IntegrationSuite }

func TestLifecycleIntegration(t *testing.T) {
	suite.Run(t, new(LifecycleSuite))
}

func (s *LifecycleSuite) TestLifecycle_FullRoundTrip() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/tool-a", "v1")))
	env.WaitForListLen(s.T(), 1, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/tool-a", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Execute(kit.NSFor("quiver-test/tool-a", "v1"), "execute", nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateRunning, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Uninstall(kit.NSFor("quiver-test/tool-a", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, 120*time.Second)

	s.Equal(http.StatusOK, tc.Remove(kit.NSFor("quiver-test/tool-a", "v1")))

	_, status := tc.GetDetail(kit.NSFor("quiver-test/tool-a", "v1"))
	s.Equal(http.StatusNotFound, status)
}

func (s *LifecycleSuite) TestLifecycle_AddIdempotency() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusCreated, tc.Add(ns))

	env.WaitForListLen(s.T(), 1, 120*time.Second)
	items, _ := tc.List()
	s.Len(items, 1)
}

func (s *LifecycleSuite) TestLifecycle_StateViaWebSocket() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	c := env.Client(s.T()) // raw client needed for WebSocket dial
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))

	conn, err := c.DialRuntime(ns)
	s.Require().NoError(err)
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(15 * time.Second))

	installDone := make(chan struct{})
	go func() {
		defer close(installDone)
		tc.Install(ns, nil)
	}()

	var states []string
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
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

	readyIdx := -1
	for i, st := range states {
		if st == string(domain.ArrowStateReady) {
			readyIdx = i
		}
	}
	s.GreaterOrEqual(readyIdx, 0, "ready state should have appeared in WebSocket stream, states: %v", states)
}

func (s *LifecycleSuite) TestLifecycle_ServiceStop() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/service-b", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	// Execute starts the long-running process (sleep 5) → running state.
	s.Equal(http.StatusAccepted, tc.Execute(ns, "execute", nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 120*time.Second)

	// Stop terminates the process → back to ready.
	s.Equal(http.StatusAccepted, tc.Stop(ns))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}

func (s *LifecycleSuite) TestLifecycle_SeedThenInstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	content := kit.ReadFixture(s.T(), "tool-a/arrow.yaml")
	s.Equal(http.StatusCreated, tc.Seed(ns, content))

	// Wait for the catalog projection to process the seed command via WebSocket stream.
	env.WaitForArrow(s.T(), ns, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}

func (s *LifecycleSuite) TestLifecycle_UpdateMethod() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-with-update", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Execute(ns, "_update", nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}

func (s *LifecycleSuite) TestLifecycle_InstalledRefInList() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	// MarkInstalled is dispatched asynchronously after install steps finish.
	// Poll until versions[0].ref is populated (typically <100ms after ready).
	var ref, installedAt string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		items, _ := tc.List()
		if len(items) == 1 && len(items[0].Versions) == 1 && items[0].Versions[0].Ref != "" {
			ref = items[0].Versions[0].Ref
			installedAt = items[0].Versions[0].InstalledAt
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	s.Equal("v1", ref)
	s.NotEmpty(installedAt)
	s.NotEqual("0001-01-01T00:00:00Z", installedAt)
}

func (s *LifecycleSuite) TestLifecycle_MarkdownArrow() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a-markdown", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	env.WaitForListLen(s.T(), 1, 120*time.Second)

	// Verify vault stored the manifest with a .md filename.
	file, err := env.Vault.GetArrow(context.Background(), domain.Namespace(ns))
	s.Require().NoError(err, "vault entry should exist after Add")
	s.True(strings.HasSuffix(file.Filename, ".md"), "vault filename should end in .md, got %q", file.Filename)

	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Execute(ns, "execute", nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 30*time.Second)
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 120*time.Second)
}

func (s *LifecycleSuite) TestLifecycle_ExecuteWithVariables() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-with-exec-var", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Execute(ns, "execute", map[string]string{"EXEC_VAR": "custom-value"}))
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 30*time.Second)
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}

func (s *LifecycleSuite) TestLifecycle_ExecuteUnknownMethod() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	_, status := tc.GetDetail(ns) // verify ready before unknown method
	s.Equal(http.StatusOK, status)

	s.Equal(http.StatusNotFound, tc.Execute(ns, "_unknownxyz", nil))

	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}
