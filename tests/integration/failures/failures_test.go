//go:build integration

package failures_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type FailuresSuite struct{ kit.IntegrationSuite }

func TestFailuresIntegration(t *testing.T) {
	suite.Run(t, new(FailuresSuite))
}

// requireInstallSettled blocks until the install leaves the installing state.
// The wait itself carries the "must not be stuck in installing" property: a
// stuck install fails here, where the previous deadline loop fell through with
// an empty state and asserted vacuously.
func (s *FailuresSuite) requireInstallSettled(tc *kit.TypedClient, ns string) {
	s.T().Helper()

	kit.WaitForDetail(
		s.T(), tc, ns, "install to leave the installing state", 120*time.Second,
		func(detail dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && detail.State != string(domain.ArrowStateInstalling)
		},
	)
}

// TestFailures_ExitOnFailureFalse_ContinuesNextStep: step 1 fails, exit_on_failure=false,
// step 2 must still run and show completed.
func (s *FailuresSuite) TestFailures_ExitOnFailureFalse_ContinuesNextStep() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-fail-continue", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))

	// Arrow must reach a terminal state (not stuck in installing).
	s.requireInstallSettled(tc, ns)

	detail := kit.WaitForLastReturn(s.T(), tc, ns, 1, 120*time.Second)
	s.Require().NotNil(detail.LastReturn, "LastReturn must be set after install attempt")
	// Steps[0] is the synthetic "Resolve dependencies" step injected by the runtime.
	s.Require().GreaterOrEqual(len(detail.LastReturn.Steps), 3, "must have at least 3 steps")
	s.Equal("failed", detail.LastReturn.Steps[1].Status,
		"step 0 (exit 1, exit_on_failure=false) must be failed")
	s.Equal("completed", detail.LastReturn.Steps[2].Status,
		"step 1 must have run and completed despite step 0 failing")
}

// TestFailures_ExitOnFailureTrue_AbortsRemaining: step 2 fails with exit_on_failure=true,
// step 3 must never run (status remains pending or is absent).
func (s *FailuresSuite) TestFailures_ExitOnFailureTrue_AbortsRemaining() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-fail-abort", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))

	// Arrow must reach a terminal state.
	s.requireInstallSettled(tc, ns)

	detail := kit.WaitForLastReturn(s.T(), tc, ns, 1, 120*time.Second)
	s.Require().NotNil(detail.LastReturn, "LastReturn must be set")
	// Steps[0] is the synthetic "Resolve dependencies" step injected by the runtime.
	s.Require().GreaterOrEqual(len(detail.LastReturn.Steps), 3, "must have at least 3 steps")
	s.Equal("completed", detail.LastReturn.Steps[1].Status, "step 0 must have completed")
	s.Equal("failed", detail.LastReturn.Steps[2].Status, "step 1 (exit 1) must be failed")
	if len(detail.LastReturn.Steps) >= 4 {
		s.NotEqual("completed", detail.LastReturn.Steps[3].Status,
			"step 2 must not have run (exit_on_failure=true aborts remaining steps)")
	}
}
