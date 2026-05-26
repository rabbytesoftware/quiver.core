//go:build integration

package guards_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type GuardsSuite struct{ kit.IntegrationSuite }

func TestGuardsIntegration(t *testing.T) {
	suite.Run(t, new(GuardsSuite))
}

// TestGuards_ExecuteWrongState: arrow is absent (never installed), execute must reject.
func (s *GuardsSuite) TestGuards_ExecuteWrongState() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	// Arrow is absent (not installed). execute on absent arrow → must reject.
	status := tc.Execute(ns, "execute", nil)
	s.GreaterOrEqual(status, 400, "execute on absent arrow must return 4xx")
	s.Less(status, 500, "execute on absent arrow must not 5xx")
}

// TestGuards_StopNonRunning: arrow is ready (not running), stop must reject.
func (s *GuardsSuite) TestGuards_StopNonRunning() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	status := tc.Stop(ns)
	s.GreaterOrEqual(status, 400, "stop on non-running arrow must return 4xx")
	s.Less(status, 500, "stop on non-running arrow must not 5xx")

	// State must not have changed.
	detail, _ := tc.GetDetail(ns)
	s.Equal(string(domain.ArrowStateReady), detail.State)
}

// TestGuards_ExecuteOnAbsentArrow: arrow not even added, execute must reject.
func (s *GuardsSuite) TestGuards_ExecuteOnAbsentArrow() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	// Arrow has never been added — GetDetail returns 404.
	status := tc.Execute(ns, "execute", nil)
	s.GreaterOrEqual(status, 400, "execute on non-existent arrow must return 4xx")
	s.Less(status, 500, "execute on non-existent arrow must not 5xx")
}

// TestGuards_UpdateWhileExecuting: running→updating is not a valid transition.
func (s *GuardsSuite) TestGuards_UpdateWhileExecuting() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	c := env.Client(s.T())
	ns := kit.NSFor("quiver-test/service-b", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	// Execute starts the long-running process (sleep 3600).
	s.Equal(http.StatusAccepted, tc.Execute(ns, "execute", nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 30*time.Second)

	// PATCH update while running — running→updating is invalid.
	resp := c.Update(ns, map[string]any{"ref": "v1"})
	resp.Body.Close()
	s.GreaterOrEqual(resp.StatusCode, 400, "update while running must return 4xx, got %d", resp.StatusCode)
	s.Less(resp.StatusCode, 500, "update while running must not 5xx, got %d", resp.StatusCode)
}
