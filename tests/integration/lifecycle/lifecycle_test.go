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

	dto "github.com/rabbytesoftware/quiver/internal/api/v0/dto"
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

func (s *LifecycleSuite) TestLifecycle_ReinstallAfterUninstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}

func (s *LifecycleSuite) TestLifecycle_StopThenExecuteAgain() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/service-b", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Execute(ns, "execute", nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 30*time.Second)

	s.Equal(http.StatusAccepted, tc.Stop(ns))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	// Execute again after stop — must accept and transition to running.
	s.Equal(http.StatusAccepted, tc.Execute(ns, "execute", nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 30*time.Second)
}

func (s *LifecycleSuite) TestLifecycle_GetDetailSeededNeverInstalled() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	content := kit.ReadFixture(s.T(), "tool-a/arrow.yaml")
	s.Equal(http.StatusCreated, tc.Seed(ns, content))
	env.WaitForArrow(s.T(), ns, 30*time.Second)

	detail, status := tc.GetDetail(ns)
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateAbsent), detail.State)
	s.Equal("quiver-test.tool-a", detail.Name)
	s.Equal("1.0.0", detail.Version)
}

func (s *LifecycleSuite) TestLifecycle_ListMixedInstalledAndNot() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	nsA := kit.NSFor("quiver-test/tool-a", "v1")
	nsB := kit.NSFor("quiver-test/service-b", "v1")

	s.Equal(http.StatusCreated, tc.Add(nsA))
	s.Equal(http.StatusCreated, tc.Add(nsB))
	env.WaitForListLen(s.T(), 2, 30*time.Second)

	s.Equal(http.StatusAccepted, tc.Install(nsA, nil))
	env.WaitForState(s.T(), nsA, domain.ArrowStateReady, 120*time.Second)

	items, status := tc.List()
	s.Equal(http.StatusOK, status)
	s.Len(items, 2)

	stateByNS := map[string]string{}
	for _, item := range items {
		if len(item.Versions) > 0 {
			stateByNS[item.Namespace] = item.Versions[0].State
		}
	}
	s.Equal(string(domain.ArrowStateReady), stateByNS["quiver-test/tool-a"],
		"installed arrow must show ready in list")
	s.Equal(string(domain.ArrowStateAbsent), stateByNS["quiver-test/service-b"],
		"non-installed arrow must show absent in list")
}

func (s *LifecycleSuite) TestLifecycle_MultistepProgress() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-multistep", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	// Poll until LastReturn is populated (set asynchronously).
	var detail dto.ArrowDetailDTO
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		detail, _ = tc.GetDetail(ns)
		if detail.LastReturn != nil && len(detail.LastReturn.Steps) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	s.Require().NotNil(detail.LastReturn, "LastReturn must be present after install")
	s.Len(detail.LastReturn.Steps, 3, "install must have 3 steps")
	for i, step := range detail.LastReturn.Steps {
		s.Equal("completed", step.Status, "step %d must be completed", i)
	}
}

func (s *LifecycleSuite) TestLifecycle_LastReturnAfterExecution() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Execute(ns, "execute", nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 30*time.Second)
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	var detail dto.ArrowDetailDTO
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		detail, _ = tc.GetDetail(ns)
		if detail.LastReturn != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	s.Require().NotNil(detail.LastReturn, "LastReturn must be present after execute")
	s.Equal("success", detail.LastReturn.Outcome)
	s.Equal("_execute", detail.LastReturn.Method)
}

func (s *LifecycleSuite) TestLifecycle_ManifestPersistsAcrossRestart() {
	home := s.T().TempDir()

	env1 := s.NewEnvWithHome(home)
	tc1 := env1.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc1.Add(ns))
	s.Equal(http.StatusAccepted, tc1.Install(ns, nil))
	env1.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
	env1.Close()

	env2 := s.NewEnvWithHome(home)
	env2.WaitForState(s.T(), ns, domain.ArrowStateReady, 30*time.Second)

	tc2 := env2.TypedClient(s.T())
	detail, status := tc2.GetDetail(ns)
	s.Equal(http.StatusOK, status)
	s.Equal("quiver-test.tool-a", detail.Name)
	s.Equal("1.0.0", detail.Version)
	s.Equal("Simple tool with no dependencies", detail.Description)
}
