//go:build integration

package stress_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/tests/integration/kit"
	"github.com/stretchr/testify/suite"
)

func TestMain(m *testing.M) { kit.Main(m) }

type StressSuite struct{ kit.IntegrationSuite }

func TestStressIntegration(t *testing.T) {
	suite.Run(t, new(StressSuite))
}

func (s *StressSuite) TestStress_DeepChain() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("dep-chain/a", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("dep-chain/a", "v1"), nil))

	for _, letter := range "abcdefghijklmnopqrstuvwxyz" {
		ns := kit.NSFor(fmt.Sprintf("dep-chain/%s", string(letter)), "v1")
		env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
	}
}

func (s *StressSuite) TestStress_WideGraph() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("dep-wide/root", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("dep-wide/root", "v1"), nil))

	env.WaitForState(s.T(), kit.NSFor("dep-wide/root", "v1"), domain.ArrowStateReady, 120*time.Second)

	for i := 1; i <= 15; i++ {
		ns := kit.NSFor(fmt.Sprintf("dep-wide/dep-%02d", i), "v1")
		env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
	}
}

func (s *StressSuite) TestStress_BulkSeed100() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	content := kit.ReadFixture(s.T(), "tool-a/arrow.yaml")

	for i := 1; i <= 100; i++ {
		ns := fmt.Sprintf("quiver.test/quiver-test/tool-bulk-%02d@v1", i)
		status := tc.Seed(ns, content)
		s.Less(status, 500, "seed %s must not cause a server error", ns)
	}

	start := time.Now()
	_, status := tc.List()
	elapsed := time.Since(start)

	s.Equal(http.StatusOK, status)
	s.Less(elapsed, 500*time.Millisecond, "List with 100 arrows should respond in <500ms")
}

func (s *StressSuite) TestStress_RestartSurvival() {
	home := s.T().TempDir()

	env1 := s.NewEnvWithHome(home)
	tc1 := env1.TypedClient(s.T())
	s.Equal(http.StatusCreated, tc1.Add(kit.NSFor("quiver-test/tool-a", "v1")))
	s.Equal(http.StatusAccepted, tc1.Install(kit.NSFor("quiver-test/tool-a", "v1"), nil))
	env1.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	env1.Close()

	env2 := s.NewEnvWithHome(home)
	tc2 := env2.TypedClient(s.T())
	detail, status := tc2.GetDetail(kit.NSFor("quiver-test/tool-a", "v1"))
	s.Equal(http.StatusOK, status, "GetDetail after restart must return 200")
	s.Equal(string(domain.ArrowStateReady), detail.State, "state should survive service restart")
}
