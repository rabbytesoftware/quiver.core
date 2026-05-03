//go:build integration

package deps_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/tests/integration/kit"
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

	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	// service-b is a long-running service; it transitions to running, not ready
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 30*time.Second)
}

func (s *DepsSuite) TestDeps_DiamondDeduplication() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("dep-diamond/root", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("dep-diamond/root", "v1"), nil))

	for _, fixture := range []string{"dep-diamond/root", "dep-diamond/left", "dep-diamond/right", "dep-diamond/shared"} {
		env.WaitForState(s.T(), kit.NSFor(fixture, "v1"), domain.ArrowStateReady, 30*time.Second)
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
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)

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
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 30*time.Second)

	s.Equal(http.StatusAccepted, tc.Stop(kit.NSFor("quiver-test/service-b", "v1")))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateReady, 30*time.Second)

	s.Equal(http.StatusAccepted, tc.Uninstall(kit.NSFor("quiver-test/composed-c", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateAbsent, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateAbsent, 30*time.Second)

	s.Equal(http.StatusOK, tc.Remove(kit.NSFor("quiver-test/composed-c", "v1")))
	s.Equal(http.StatusOK, tc.Remove(kit.NSFor("quiver-test/tool-a", "v1")))

	_, status := tc.GetDetail(kit.NSFor("quiver-test/tool-a", "v1"))
	s.Equal(http.StatusNotFound, status)
}

func (s *DepsSuite) TestDeps_OrphanCleanup() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/composed-c", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/composed-c", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 30*time.Second)

	s.Equal(http.StatusAccepted, tc.Uninstall(kit.NSFor("quiver-test/composed-c", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateAbsent, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateAbsent, 30*time.Second)
}

func (s *DepsSuite) TestStop_CascadesOrphanedService() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/composed-c", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/composed-c", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 30*time.Second)

	s.Equal(http.StatusAccepted, tc.Execute(kit.NSFor("quiver-test/composed-c", "v1"), domain.MethodExecute, nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateRunning, 30*time.Second)

	s.Equal(http.StatusAccepted, tc.Stop(kit.NSFor("quiver-test/composed-c", "v1")))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 30*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateReady, 30*time.Second)
}
