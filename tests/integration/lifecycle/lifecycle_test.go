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

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
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
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateRunning, 120*time.Second)
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

// quiver.test appears in no platform table, so no configured branch list can
// answer a refless add. The ref comes off the remote's HEAD instead.
func (s *LifecycleSuite) TestLifecycle_AddReflessOnAnUnlistedPlatform() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add("quiver.test/quiver-test/tool-a"))
	env.WaitForListLen(s.T(), 1, 120*time.Second)

	items, _ := tc.List()
	s.Require().Len(items, 1)
	s.Require().Len(items[0].Versions, 1)
	s.NotEmpty(items[0].Versions[0].Version, "a refless add must land on a real ref")
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

	// MarkInstalled is dispatched asynchronously after install steps finish, and
	// reaches the list through its own projection, so `ready` does not imply it.
	items := kit.WaitForList(
		s.T(), tc, "the installed ref to reach the catalog list", 120*time.Second,
		func(items []dto.ArrowListItemDTO, status int) bool {
			return status == http.StatusOK &&
				len(items) == 1 &&
				len(items[0].Versions) == 1 &&
				items[0].Versions[0].Ref != ""
		},
	)

	s.Equal("v1", items[0].Versions[0].Ref)
	s.NotEmpty(items[0].Versions[0].InstalledAt)
	s.NotEqual("0001-01-01T00:00:00Z", items[0].Versions[0].InstalledAt)
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
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 120*time.Second)
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
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 120*time.Second)
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
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Stop(ns))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	// Execute again after stop — must accept and transition to running.
	s.Equal(http.StatusAccepted, tc.Execute(ns, "execute", nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 120*time.Second)
}

func (s *LifecycleSuite) TestLifecycle_GetDetailSeededNeverInstalled() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	content := kit.ReadFixture(s.T(), "tool-a/arrow.yaml")
	s.Equal(http.StatusCreated, tc.Seed(ns, content))
	env.WaitForArrow(s.T(), ns, 120*time.Second)

	detail, status := tc.GetDetail(ns)
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateAbsent), detail.State)
	s.Equal("quiver-test.tool-a", detail.Name)
	s.Equal("v1", detail.Version)
}

func (s *LifecycleSuite) TestLifecycle_ListMixedInstalledAndNot() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	nsA := kit.NSFor("quiver-test/tool-a", "v1")
	nsB := kit.NSFor("quiver-test/service-b", "v1")

	s.Equal(http.StatusCreated, tc.Add(nsA))
	s.Equal(http.StatusCreated, tc.Add(nsB))
	env.WaitForListLen(s.T(), 2, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Install(nsA, nil))
	env.WaitForState(s.T(), nsA, domain.ArrowStateReady, 120*time.Second)

	items, status := tc.List()
	s.Equal(http.StatusOK, status)
	s.Len(items, 2)

	stateByNS := map[string]string{}
	for _, item := range items {
		if len(item.Versions) > 0 {
			stateByNS[item.Namespace] = string(item.Versions[0].State)
		}
	}
	bareA := string(domain.Namespace(nsA).BareNamespace())
	bareB := string(domain.Namespace(nsB).BareNamespace())
	s.Equal(string(domain.ArrowStateReady), stateByNS[bareA],
		"installed arrow must show ready in list")
	s.Equal(string(domain.ArrowStateAbsent), stateByNS[bareB],
		"non-installed arrow must show absent in list")
}

func (s *LifecycleSuite) TestLifecycle_MultistepProgress() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-multistep", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	detail := kit.WaitForLastReturn(s.T(), tc, ns, 1, 120*time.Second)

	// The runtime prepends a synthetic "Resolve dependencies" step at index 0.
	s.Require().NotNil(detail.LastReturn, "LastReturn must be present after install")
	s.Require().Len(detail.LastReturn.Steps, 4, "install must have 4 steps (1 dep-resolve + 3 user)")
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
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 120*time.Second)
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	detail := kit.WaitForLastReturn(s.T(), tc, ns, 0, 120*time.Second)

	s.Require().NotNil(detail.LastReturn, "LastReturn must be present after execute")
	s.Equal("success", detail.LastReturn.Outcome)
	s.Equal("_execute", detail.LastReturn.Method)
}

func (s *LifecycleSuite) TestLifecycle_RuntimeFreshAfterReAdd() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	// Confirm LastReturn is populated after install.
	detail := kit.WaitForLastReturn(s.T(), tc, ns, 0, 120*time.Second)
	s.Require().NotNil(detail.LastReturn, "LastReturn must be set after install")

	s.Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 120*time.Second)
	s.Equal(http.StatusOK, tc.Remove(ns))

	// Re-add and verify the runtime starts fresh — no stale LastReturn.
	s.Equal(http.StatusCreated, tc.Add(ns))
	freshDetail := kit.WaitForDetail(
		s.T(), tc, ns, "the re-added arrow to be served again", 120*time.Second,
		func(_ dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK
		},
	)
	s.Nil(freshDetail.LastReturn, "LastReturn must be nil after removing and re-adding the arrow")
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
	tc2 := env2.TypedClient(s.T())

	// The event store is persisted to disk — ready state survives a restart.
	// No WS event fires (no transition occurred), so read state directly from REST.
	detail, status := tc2.GetDetail(ns)
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateReady), detail.State, "arrow must be ready after restart")
	s.Equal("quiver-test.tool-a", detail.Name)
	s.Equal("v1", detail.Version)
	s.Equal("Simple tool with no dependencies", detail.Description)
}
