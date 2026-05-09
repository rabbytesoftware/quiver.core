//go:build integration

package oracle_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type OracleSuite struct{ kit.IntegrationSuite }

func TestOracleIntegration(t *testing.T) {
	suite.Run(t, new(OracleSuite))
}

func (s *OracleSuite) TestOracle_ConsistencyAfterInstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)

	kit.AssertConsistency(s.T(), env, tc, ns, domain.ArrowStateReady, true, true)
}

func (s *OracleSuite) TestOracle_ConsistencyAfterUninstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)
	s.Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 60*time.Second)

	// After uninstall: state=absent, list still shows it, vault entry preserved.
	kit.AssertConsistency(s.T(), env, tc, ns, domain.ArrowStateAbsent, true, true)
}

func (s *OracleSuite) TestOracle_ConsistencyAfterRemove() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)
	s.Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 60*time.Second)
	s.Equal(http.StatusOK, tc.Remove(ns))

	// After remove: 404, not in list, vault entry preserved (cache).
	kit.AssertConsistency(s.T(), env, tc, ns, "", false, true)
}

func (s *OracleSuite) TestOracle_ProjectionLag() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))

	// Arrow must appear in List within 1s.
	deadline := time.Now().Add(1 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		items, status := tc.List()
		if status == http.StatusOK {
			for _, item := range items {
				if item.Namespace == string(domain.Namespace(ns).BareNamespace()) {
					found = true
					break
				}
			}
		}
		if found {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.True(found, "arrow must appear in List within 1s of Add (projection lag)")
}

func (s *OracleSuite) TestOracle_PhantomDepEdges() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	nsA := kit.NSFor("quiver-test/composed-c", "v1")

	s.Equal(http.StatusCreated, tc.Add(nsA))
	s.Equal(http.StatusAccepted, tc.Install(nsA, nil))
	env.WaitForState(s.T(), nsA, domain.ArrowStateReady, 120*time.Second)
	s.Equal(http.StatusAccepted, tc.Uninstall(nsA, nil))
	env.WaitForState(s.T(), nsA, domain.ArrowStateAbsent, 60*time.Second)
	s.Equal(http.StatusOK, tc.Remove(nsA))

	// Vault entry is preserved after remove (manifest cache).
	_, vaultErr := env.Vault.GetArrow(context.Background(), domain.Namespace(nsA))
	s.NoError(vaultErr, "vault entry must be preserved after remove")

	// Re-add with no deps — old dep edges must be gone so this succeeds.
	content := kit.ReadFixture(s.T(), "tool-a/arrow.yaml")
	s.Equal(http.StatusCreated, tc.Seed(nsA, content))
	env.WaitForArrow(s.T(), nsA, 30*time.Second)
}
