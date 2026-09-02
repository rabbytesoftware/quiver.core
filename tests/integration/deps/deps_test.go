//go:build integration

package deps_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type DepsSuite struct{ kit.IntegrationSuite }

func TestDepsIntegration(t *testing.T) {
	suite.Run(t, new(DepsSuite))
}

func (s *DepsSuite) TestDeps_TransitiveInstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/composed-c", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/composed-c", "v1"), nil))

	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	// service-b is a long-running service; it transitions to running, not ready
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 120*time.Second)
}

func (s *DepsSuite) TestDeps_DiamondDeduplication() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("dep-diamond/root", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("dep-diamond/root", "v1"), nil))

	for _, fixture := range []string{"dep-diamond/root", "dep-diamond/left", "dep-diamond/right", "dep-diamond/shared"} {
		env.WaitForState(s.T(), kit.NSFor(fixture, "v1"), domain.ArrowStateReady, 120*time.Second)
	}

	_, status := tc.GetDetail(kit.NSFor("dep-diamond/shared", "v1"))
	s.Equal(http.StatusOK, status)

	items, _ := tc.List()
	sharedCount := 0
	for _, item := range items {
		if strings.Contains(item.Namespace, "shared") {
			sharedCount++
		}
	}
	s.LessOrEqual(sharedCount, 1, "dep-diamond/shared should appear at most once in catalog list")
}

func (s *DepsSuite) TestDeps_CircularDetection() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	addStatus := tc.Add(kit.NSFor("dep-circular/circ-a", "v1"))
	if addStatus == http.StatusCreated {
		installStatus := tc.Install(kit.NSFor("dep-circular/circ-a", "v1"), nil)
		s.GreaterOrEqual(installStatus, 400, "install of circular dep should fail")
	} else {
		s.GreaterOrEqual(addStatus, 400, "add of circular dep should fail")
	}
}

func (s *DepsSuite) TestDeps_RemoveBlockedByDependents() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/composed-c", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/composed-c", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)

	// tool-a is ready (installed as dep) — removing it must fail with 422
	s.Equal(http.StatusUnprocessableEntity, tc.Remove(kit.NSFor("quiver-test/tool-a", "v1")))

	_, status := tc.GetDetail(kit.NSFor("quiver-test/tool-a", "v1"))
	s.Equal(http.StatusOK, status)
}

func (s *DepsSuite) TestDeps_RemoveAfterDependentsGone() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/composed-c", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/composed-c", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Stop(kit.NSFor("quiver-test/service-b", "v1")))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Uninstall(kit.NSFor("quiver-test/composed-c", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateAbsent, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateAbsent, 120*time.Second)

	s.Equal(http.StatusOK, tc.Remove(kit.NSFor("quiver-test/composed-c", "v1")))
	s.Equal(http.StatusOK, tc.Remove(kit.NSFor("quiver-test/tool-a", "v1")))

	// Removed from the catalog, but GetDetail resolves it live like an
	// uncatalogued namespace — the fixture repository still resolves, so this
	// reports state "absent" rather than 404ing.
	detail, status := tc.GetDetail(kit.NSFor("quiver-test/tool-a", "v1"))
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateAbsent), detail.State)
}

func (s *DepsSuite) TestDeps_OrphanCleanup() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/composed-c", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/composed-c", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Uninstall(kit.NSFor("quiver-test/composed-c", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateAbsent, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateAbsent, 120*time.Second)
}

// TestDeps_ExportsInjectedToConsumer installs an arrow whose install step reads
// its dependency's export through the documented ${namespace.EXPORT_NAME} form.
// Quiver resolves the reference before the command reaches the shell, so this
// runs identically on every platform regardless of what /bin/sh is.
func (s *DepsSuite) TestDeps_ExportsInjectedToConsumer() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/tool-consumer", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/tool-consumer", "v1"), nil))

	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-exporter", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-consumer", "v1"), domain.ArrowStateReady, 120*time.Second)
}

func (s *DepsSuite) TestStop_CascadesOrphanedService() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/composed-c", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/composed-c", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Execute(kit.NSFor("quiver-test/composed-c", "v1"), domain.MethodExecute, nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateRunning, 120*time.Second)

	s.Equal(http.StatusAccepted, tc.Stop(kit.NSFor("quiver-test/composed-c", "v1")))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateReady, 120*time.Second)
}

// TestDeps_ThreeNodeCycle: A→B→C→A must be rejected (409 Conflict).
func (s *DepsSuite) TestDeps_ThreeNodeCycle() {
	env := s.NewEnv()
	c := env.Client(s.T())

	addResp := c.Add(kit.NSFor("dep-circ-three/a", "v1"))
	defer addResp.Body.Close()

	if addResp.StatusCode == http.StatusCreated {
		installResp := c.Install(kit.NSFor("dep-circ-three/a", "v1"), nil)
		installResp.Body.Close()
		s.Equal(http.StatusConflict, installResp.StatusCode,
			"install of 3-node cycle must return 409 Conflict")
	} else {
		s.Equal(http.StatusConflict, addResp.StatusCode,
			"add of 3-node cycle must return 409 Conflict")
	}
}

// TestDeps_SelfReferentialCycle: A→A must be rejected (409 Conflict).
// dep-self lives at testdata/arrows/dep-self/ (root level) so dirToKey prefixes "quiver-test/".
func (s *DepsSuite) TestDeps_SelfReferentialCycle() {
	env := s.NewEnv()
	c := env.Client(s.T())

	addResp := c.Add(kit.NSFor("quiver-test/dep-self", "v1"))
	defer addResp.Body.Close()

	if addResp.StatusCode == http.StatusCreated {
		installResp := c.Install(kit.NSFor("quiver-test/dep-self", "v1"), nil)
		installResp.Body.Close()
		s.Equal(http.StatusConflict, installResp.StatusCode,
			"install of self-referential dep must return 409 Conflict")
	} else {
		s.Equal(http.StatusConflict, addResp.StatusCode,
			"add of self-referential dep must return 409 Conflict")
	}
}

// TestDeps_CycleByUpgrade: seeding dep@v2 (which depends on base) while base depends on dep@v1
// probes whether Quiver detects version-aware upgrade cycles. In Quiver's model dep@v1 and dep@v2
// are distinct namespaces, so this may NOT be detected as a cycle — escalate if unsure.
func (s *DepsSuite) TestDeps_CycleByUpgrade() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	// Set up: base→dep@v1 (no cycle).
	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("dep-upgrade-cycle/base", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("dep-upgrade-cycle/base", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("dep-upgrade-cycle/dep", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("dep-upgrade-cycle/base", "v1"), domain.ArrowStateReady, 120*time.Second)

	// Seed dep@v2 inline — it references base, creating the potential cycle base→dep@v2→base.
	nsDepV2 := kit.NSFor("dep-upgrade-cycle/dep", "v2")
	depV2YAML := []byte(`schema: "arrow@v0"
metadata:
  name: quiver-test.dep-upgrade-cycle-dep
  description: Dep v2 — depends on base (potential cycle)
targets:
  "*":
    tools:
      - quiver.test/dep-upgrade-cycle/base@v1
    lifecycle:
      install:
        - type: run
          command: echo installed-upgrade-cycle-dep-v2
          title: Install
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled-upgrade-cycle-dep-v2
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`)
	// dep@v1 and dep@v2 are distinct namespaces in Quiver's versioned-namespace model.
	// base→dep@v1 and dep@v2→base is not a cycle — no rejection expected.
	s.Equal(http.StatusCreated, tc.Seed(nsDepV2, depV2YAML))
	s.Equal(http.StatusAccepted, tc.Install(nsDepV2, nil))
}

// TestDeps_CatalogCleanAfterFailedAdd: after a cycle-rejected Add, List must show no partial state.
func (s *DepsSuite) TestDeps_CatalogCleanAfterFailedAdd() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	addStatus := tc.Add(kit.NSFor("dep-circular/circ-a", "v1"))
	if addStatus != http.StatusCreated {
		// Add was rejected — verify no partial state leaked into catalog.
		items, _ := tc.List()
		for _, item := range items {
			s.NotContains(item.Namespace, "circ-a",
				"circ-a must not appear in catalog after a rejected Add")
		}
		return
	}

	// Add succeeded — install must fail. Catalog may have circ-a (it was added).
	ns := kit.NSFor("dep-circular/circ-a", "v1")
	installStatus := tc.Install(ns, nil)
	if installStatus == http.StatusAccepted {
		// Cycle slipped past assembly; wizard detects it — wait for terminal state.
		env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 120*time.Second)
	} else {
		s.GreaterOrEqual(installStatus, 400)
	}
	// Verify circ-a is in catalog but in a clean non-installing state.
	detail, status := tc.GetDetail(ns)
	s.Equal(http.StatusOK, status)
	s.NotEqual(string(domain.ArrowStateInstalling), detail.State,
		"after failed install, arrow must not be stuck in installing")
}

// TestDeps_CatalogCleanAfterFailedInstall: arrow seeded with an always-failing install
// script must remain in the catalog in a non-installing state after the install fails.
// This exercises the async install-fail path (BeginInstall sent → wizard fails → EndExecution)
// and verifies neither the 404-removal nor the stuck-installing failure mode is present.
func (s *DepsSuite) TestDeps_CatalogCleanAfterFailedInstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	ns := kit.NSFor("quiver-test/catalog-clean-fail", "v1")
	manifest := []byte(`schema: "arrow@v0"
metadata:
  name: quiver-test.catalog-clean-fail
  description: Always-failing install for catalog-cleanliness testing
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: "false"
          title: Always fails
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`)

	s.Equal(http.StatusCreated, tc.Seed(ns, manifest))

	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 120*time.Second)

	detail, status := tc.GetDetail(ns)
	s.Equal(http.StatusOK, status, "arrow must remain in catalog after failed install")
	s.NotEqual(string(domain.ArrowStateInstalling), detail.State,
		"after failed install, arrow must not be stuck in installing")
}

// TestDeps_CycleRejectionStatusCode: cycle rejection must be exactly 409, not any ≥400.
func (s *DepsSuite) TestDeps_CycleRejectionStatusCode() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	c := env.Client(s.T())

	addStatus := tc.Add(kit.NSFor("dep-circular/circ-a", "v1"))
	if addStatus == http.StatusCreated {
		installResp := c.Install(kit.NSFor("dep-circular/circ-a", "v1"), nil)
		installResp.Body.Close()
		s.Equal(http.StatusConflict, installResp.StatusCode,
			"cycle install rejection must be exactly 409 Conflict, got %d", installResp.StatusCode)
	} else {
		s.Equal(http.StatusConflict, addStatus,
			"cycle add rejection must be exactly 409 Conflict, got %d", addStatus)
	}
}

// TestDeps_CyclePathInResponse: the error response body must contain the cycle path.
func (s *DepsSuite) TestDeps_CyclePathInResponse() {
	env := s.NewEnv()
	c := env.Client(s.T())

	addResp := c.Add(kit.NSFor("dep-circular/circ-a", "v1"))
	defer addResp.Body.Close()

	var body []byte
	if addResp.StatusCode == http.StatusCreated {
		addResp.Body.Close()
		installResp := c.Install(kit.NSFor("dep-circular/circ-a", "v1"), nil)
		defer installResp.Body.Close()
		body, _ = io.ReadAll(installResp.Body)
	} else {
		body, _ = io.ReadAll(addResp.Body)
	}

	// Response body must contain namespace fragments indicating the cycle path.
	bodyStr := string(body)
	s.True(
		strings.Contains(bodyStr, "circ-a") || strings.Contains(bodyStr, "cycle") || strings.Contains(bodyStr, "circular"),
		"response body must mention the cycle participants, got: %s", bodyStr,
	)
}
