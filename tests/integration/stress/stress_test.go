//go:build integration

package stress_test

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
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

	// Correctness at scale, not latency: a wall-clock bound here measures the
	// machine, not the code, and flakes whenever the suite runs under load.
	// Read-path regressions are `make bench`'s job — it has a recorded baseline
	// and a 1.25x threshold, which this assertion never had.
	items, status := tc.List()

	s.Equal(http.StatusOK, status)
	s.Len(items, 100, "every seeded arrow must be listed")
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

func (s *StressSuite) TestStress_RestartWith100Arrows() {
	home := s.T().TempDir()
	env1 := s.NewEnvWithHome(home)
	tc1 := env1.TypedClient(s.T())
	content := kit.ReadFixture(s.T(), "tool-a/arrow.yaml")

	nss := make([]string, 100)
	for i := range nss {
		nss[i] = fmt.Sprintf("quiver.test/quiver-bench/tool-%03d@v1", i+1)
		tc1.Seed(nss[i], content)
	}
	env1.WaitForCatalogLen(s.T(), 100, 120*time.Second)
	env1.Close()

	env2 := s.NewEnvWithHome(home)
	items, status := env2.TypedClient(s.T()).List()
	s.Equal(http.StatusOK, status)
	s.Len(items, 100, "all 100 arrows must survive restart (catalog replay at scale)")
	env2.Close()
}

func (s *StressSuite) TestStress_EventStoreGrowth() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))

	for cycle := 0; cycle < 20; cycle++ {
		s.Equal(http.StatusAccepted, tc.Install(ns, nil))
		env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)
		s.Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
		env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, 60*time.Second)
	}

	// What 20 cycles actually risk is a wrong aggregate, not a slow one: the
	// event stream is now deep enough that a snapshot or replay defect would
	// surface as stale state rather than latency. Assert the state, and leave
	// read-path timing to `make bench`, which has a baseline to compare against.
	// MarkUninstalled is dispatched asynchronously after the uninstall steps
	// finish, so the absent transition does not imply the stamp is off yet.
	detail := kit.WaitForDetail(
		s.T(), tc, ns, "installed_at to be cleared", 60*time.Second,
		func(detail dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && detail.InstalledAt == ""
		},
	)
	s.Equal(ns, detail.Namespace)
	s.Equal(string(domain.ArrowStateAbsent), detail.State,
		"after an even number of install/uninstall cycles the replayed state must be absent")
	s.Empty(detail.InstalledAt, "an uninstalled arrow must carry no install stamp")
}

func (s *StressSuite) TestStress_50ConcurrentInstalls() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	content := kit.ReadFixture(s.T(), "tool-a/arrow.yaml")

	const N = 50
	namespaces := make([]string, N)
	for i := range namespaces {
		namespaces[i] = fmt.Sprintf("quiver.test/quiver-bench/concurrent-%02d@v1", i+1)
		tc.Seed(namespaces[i], content)
	}
	env.WaitForCatalogLen(s.T(), N, 60*time.Second)

	var wg sync.WaitGroup
	for _, ns := range namespaces {
		wg.Add(1)
		ns := ns
		go func() {
			defer wg.Done()
			tc.Install(ns, nil)
		}()
	}
	wg.Wait()

	for _, ns := range namespaces {
		env.WaitForState(s.T(), ns, domain.ArrowStateReady, 180*time.Second)
	}
}
