//go:build integration

package reliability_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type ReliabilitySuite struct{ kit.IntegrationSuite }

func TestReliabilityIntegration(t *testing.T) {
	suite.Run(t, new(ReliabilitySuite))
}

// TestReliability_StepTimeout: install an arrow whose step sleeps 5s with 200ms timeout.
// Arrow must reach a terminal non-installing state (absent after failed install).
func (s *ReliabilitySuite) TestReliability_StepTimeout() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-with-timeout", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	env.WaitForArrow(s.T(), ns, 30*time.Second)
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))

	// After a timeout the install fails and the arrow returns to absent.
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 60*time.Second)
}

// TestReliability_ServiceSelfTermination: execute step exits immediately (exit 0).
// Quiver must detect process death and return the arrow to ready.
func (s *ReliabilitySuite) TestReliability_ServiceSelfTermination() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-self-terminating", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	env.WaitForArrow(s.T(), ns, 30*time.Second)
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)

	s.Equal(http.StatusAccepted, tc.Execute(ns, "execute", nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, 15*time.Second)
	// Process exits immediately — Quiver must detect it and return to ready.
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 30*time.Second)
}

// TestReliability_NetbridgePortConflict: two arrows declare the same netbridge port.
// Declared ports are suggestions — Quiver allocates a free port if the requested
// one is in use. Both installs must succeed.
func (s *ReliabilitySuite) TestReliability_NetbridgePortConflict() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	nsA := kit.NSFor("quiver-test/tool-netbridge-a", "v1")
	nsB := kit.NSFor("quiver-test/tool-netbridge-b", "v1")

	s.Equal(http.StatusCreated, tc.Add(nsA))
	s.Equal(http.StatusAccepted, tc.Install(nsA, nil))
	env.WaitForState(s.T(), nsA, domain.ArrowStateReady, 60*time.Second)

	s.Equal(http.StatusCreated, tc.Add(nsB))
	s.Equal(http.StatusAccepted, tc.Install(nsB, nil))
	env.WaitForState(s.T(), nsB, domain.ArrowStateReady, 60*time.Second)
}

// TestReliability_RapidChurn: install/uninstall the same arrow 10 times.
// After all cycles the arrow must be absent with no dangling runtime.
func (s *ReliabilitySuite) TestReliability_RapidChurn() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))

	for i := 0; i < 10; i++ {
		s.Equal(http.StatusAccepted, tc.Install(ns, nil))
		env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)
		s.Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
		env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 60*time.Second)
	}

	detail, status := tc.GetDetail(ns)
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateAbsent), detail.State)
	s.Nil(detail.ActiveRun, "no dangling runtime after churn")
}

// TestReliability_AddRemoveWithoutInstall: Add then Remove without installing.
// GetDetail must 404.
func (s *ReliabilitySuite) TestReliability_AddRemoveWithoutInstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	env.WaitForArrow(s.T(), ns, 30*time.Second)
	s.Equal(http.StatusOK, tc.Remove(ns))

	_, status := tc.GetDetail(ns)
	s.Equal(http.StatusNotFound, status)
}

// TestReliability_OrphanDepNotCleaned: install A (auto-installs deps).
// Explicitly install dep B (now user-owned). Uninstall A.
// B must remain ready — user-owned arrows are never orphan-cleaned.
func (s *ReliabilitySuite) TestReliability_OrphanDepNotCleaned() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	// composed-c depends on quiver-test/tool-a (tool) and quiver-test/service-b (service).
	nsA := kit.NSFor("quiver-test/composed-c", "v1")
	nsDep := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(nsA))
	s.Equal(http.StatusAccepted, tc.Install(nsA, nil))
	env.WaitForState(s.T(), nsDep, domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), nsA, domain.ArrowStateReady, 120*time.Second)

	// Explicitly install the dep — it becomes user-owned.
	s.Equal(http.StatusAccepted, tc.Install(nsDep, nil))
	env.WaitForState(s.T(), nsDep, domain.ArrowStateReady, 60*time.Second)

	// Uninstall A — dep must survive.
	s.Equal(http.StatusAccepted, tc.Uninstall(nsA, nil))
	env.WaitForState(s.T(), nsA, domain.ArrowStateAbsent, 60*time.Second)

	detail, status := tc.GetDetail(nsDep)
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateReady), detail.State,
		"user-owned dep must remain ready after parent uninstalls")
}

// TestReliability_StepWithNoTimeout: validates a manifest with no timeout is valid,
// and a manifest with a malformed timeout (5ms) triggers the invalid_timeout rule.
func (s *ReliabilitySuite) TestReliability_StepWithNoTimeout() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	body := []byte(`schema: "arrow@v0"
metadata:
  name: quiver-test.no-timeout
  version: 1.0.0
  description: Step with no timeout
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: echo installed
          title: Install
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          exit_on_failure: false
`)
	ns := fmt.Sprintf("%s", kit.NSFor("quiver-test/no-timeout", "v1"))

	// Validate first — expect no timeout errors.
	result, status := tc.Validate(ns, body)
	s.Equal(http.StatusOK, status)
	for _, e := range result.Errors {
		s.NotContains(e.Rule, "timeout",
			"missing timeout should not be a validation error")
	}

	// Seed and install — must complete without hanging.
	s.Equal(http.StatusCreated, tc.Seed(ns, body))
	env.WaitForArrow(s.T(), ns, 30*time.Second)
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 30*time.Second)

	// Verify that a malformed timeout triggers the invalid_timeout rule.
	badBody := []byte(strings.Replace(string(body),
		"exit_on_failure: true",
		"timeout: 5ms\n          exit_on_failure: true", 1))
	badResult, _ := tc.Validate(ns, badBody)
	hasInvalidTimeoutRule := false
	for _, e := range badResult.Errors {
		if e.Rule == "invalid_timeout" {
			hasInvalidTimeoutRule = true
		}
	}
	s.True(hasInvalidTimeoutRule, "malformed timeout '5ms' must trigger invalid_timeout rule")
}
