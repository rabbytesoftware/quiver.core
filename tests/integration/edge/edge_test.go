//go:build integration

package edge_test

import (
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	dto "github.com/rabbytesoftware/quiver/internal/api/v0/dto"
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

func (s *EdgeSuite) TestEdge_OSSpecificTargetSelected() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-os-specific", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	var detail dto.ArrowDetailDTO
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		detail, _ = tc.GetDetail(ns)
		if detail.LastReturn != nil && len(detail.LastReturn.Steps) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.Require().NotNil(detail.LastReturn, "LastReturn must be set after install")
	s.Require().NotEmpty(detail.LastReturn.Steps, "install must have steps")

	var expectedTitle string
	switch runtime.GOOS {
	case "darwin":
		expectedTitle = "Install (macOS)"
	case "linux":
		expectedTitle = "Install (Linux)"
	default:
		s.T().Skipf("tool-os-specific has no target for GOOS=%s", runtime.GOOS)
	}
	// Steps[0] is the synthetic "Resolve dependencies" step; user steps start at [1].
	s.Require().Greater(len(detail.LastReturn.Steps), 1, "must have user steps beyond dep-resolve")
	s.Equal(expectedTitle, detail.LastReturn.Steps[1].Title,
		"step title must match current OS target")
}

func (s *EdgeSuite) TestEdge_CustomMethodExecutes() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-with-custom-method", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Execute(ns, "greet", nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 30*time.Second)
}

func (s *EdgeSuite) TestEdge_TagsInDetail() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-with-tags", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	env.WaitForArrow(s.T(), ns, 30*time.Second)

	detail, status := tc.GetDetail(ns)
	s.Equal(http.StatusOK, status)
	s.Contains(detail.Tags, "integration-tag-a")
	s.Contains(detail.Tags, "integration-tag-b")
}

func (s *EdgeSuite) TestEdge_FetchStep() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-with-fetch", "v1")

	fetchURL := kit.ServeFetch(s.T(), []byte("binary-content"))
	content := kit.RenderFixture(s.T(), "tool-with-fetch/arrow.yaml", map[string]string{
		"FETCH_URL": fetchURL,
	})
	s.Equal(http.StatusCreated, tc.Seed(ns, content))
	env.WaitForArrow(s.T(), ns, 30*time.Second)

	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}

func (s *EdgeSuite) TestEdge_DependenciesStep() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	root := kit.NSFor("quiver-test/tool-with-deps-step", "v1")
	dep := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(root))
	s.Equal(http.StatusAccepted, tc.Install(root, nil))

	env.WaitForState(s.T(), dep, domain.ArrowStateReady, 30*time.Second)
	env.WaitForState(s.T(), root, domain.ArrowStateReady, 120*time.Second)
}

func (s *EdgeSuite) TestEdge_ValidateManifestRuleViolation() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1") // namespace is arbitrary for validate

	// Step with invalid timeout format (ms not supported) — triggers timeout_format rule.
	body := []byte(`schema: "arrow@v0"
metadata:
  name: bad-arrow
  version: 1.0.0
  description: Test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: echo hi
          title: Bad Timeout
          timeout: 200ms
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo bye
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`)
	result, status := tc.Validate(ns, body)
	s.Equal(http.StatusUnprocessableEntity, status)
	s.False(result.Valid, "manifest with invalid timeout format must be invalid")
	s.Require().NotEmpty(result.Errors)

	ruleNames := make([]string, len(result.Errors))
	for i, e := range result.Errors {
		ruleNames[i] = e.Rule
	}
	s.Contains(ruleNames, "invalid_timeout",
		"errors must include invalid_timeout rule, got: %v", ruleNames)
}
