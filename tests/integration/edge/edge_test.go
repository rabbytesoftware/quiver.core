//go:build integration

package edge_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/tests/integration/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type EdgeSuite struct{ kit.IntegrationSuite }

func TestEdgeIntegration(t *testing.T) {
	suite.Run(t, new(EdgeSuite))
}

func (s *EdgeSuite) TestEdge_InstallWhileAlreadyInstalling() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	c := env.Client(s.T())
	ns := kit.NSFor("quiver-test/tool-a-slow", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))

	// Second install while already installing is idempotent — returns 202 (accepted).
	// The in-flight install completes normally; the second call is a no-op.
	resp2 := c.Install(ns, nil)
	defer resp2.Body.Close()
	s.NotEqual(http.StatusInternalServerError, resp2.StatusCode,
		"second install while installing must not cause a server error")

	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}

func (s *EdgeSuite) TestEdge_ExecuteWhileInstalling() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	c := env.Client(s.T())
	ns := kit.NSFor("quiver-test/tool-a-slow", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))

	resp2 := c.Execute(ns, "execute", nil)
	defer resp2.Body.Close()
	s.GreaterOrEqual(resp2.StatusCode, 400)

	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}

func (s *EdgeSuite) TestEdge_RemoveWhileInstalling() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	c := env.Client(s.T())
	ns := kit.NSFor("quiver-test/tool-a-slow", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))

	resp2 := c.Remove(ns)
	defer resp2.Body.Close()
	s.GreaterOrEqual(resp2.StatusCode, 400)

	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}

func (s *EdgeSuite) TestEdge_MalformedYAML() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/malformed", "v1")

	content := kit.ReadFixture(s.T(), "malformed/arrow.yaml")
	status := tc.Seed(ns, content)
	s.GreaterOrEqual(status, 400, "malformed YAML seed must fail with 4xx")
	s.Less(status, 500, "malformed YAML seed must not be a server error")

	_, getStatus := tc.GetDetail(ns)
	s.Equal(http.StatusNotFound, getStatus)
}

func (s *EdgeSuite) TestEdge_RulesetViolation() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/invalid-ruleset", "v1")

	content := kit.ReadFixture(s.T(), "invalid-ruleset/arrow.yaml")
	result, status := tc.Validate(ns, content)

	s.Equal(http.StatusUnprocessableEntity, status)
	s.False(result.Valid, "response data.valid must be false for ruleset violation")
	s.NotEmpty(result.Errors, "response data.errors must contain at least one error")
}

func (s *EdgeSuite) TestEdge_NoTargetForCurrentOS() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	c := env.Client(s.T())
	ns := kit.NSFor("quiver-test/no-current-os", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))

	resp := c.Install(ns, nil)
	defer resp.Body.Close()
	s.GreaterOrEqual(resp.StatusCode, 400, "install with no target for current OS must fail")
}

func (s *EdgeSuite) TestEdge_MissingVariablesAllowedCurrently() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/missing-vars", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))

	// Production does not validate required variables at request time.
	// Install is accepted (202) and runs with variable unset.
	// TODO: enforce required variable validation to return 4xx here.
	status := tc.Install(ns, nil)
	s.Less(status, 500, "install must not cause a server error")
}

func (s *EdgeSuite) TestEdge_VariableDefaultApplied() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-with-default-var", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}

func (s *EdgeSuite) TestEdge_ValidateValidManifest() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	content := kit.ReadFixture(s.T(), "tool-a/arrow.yaml")
	result, status := tc.Validate(ns, content)

	s.Equal(http.StatusOK, status)
	s.True(result.Valid, "valid manifest must have data.valid = true")
	s.NotEmpty(result.SupportedPlatforms, "valid manifest must list supported_platforms")
	s.Empty(result.Errors, "valid manifest must have no errors")
}
