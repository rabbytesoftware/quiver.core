//go:build integration

package fletcher_test

import (
	"net/http"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// A drafted stable row that is uninstalled, advanced by PATCH (nothing is
// installed from it, spec §8.1) and installed again must install the release
// it now resolves to: the release-asset URL is built from ${REF}.
func (s *FletcherSuite) TestAdversarial_AdvancedUninstalledRow_InstallsTheNewRelease() {
	storer := s.releasedFixture(releasingFixture, "v1.0.0")
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(releasingFixture, "stable")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, waitTimeout)
	s.exposedDir(env, "tool", "tool v1.0.0")
	s.Require().Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, waitTimeout)

	s.Repos.Mutate(func() { kit.TagHead(s.T(), storer, "v1.1.0") })
	_, status := tc.CheckAvailable(ns)
	s.Require().Equal(http.StatusOK, status)
	detail, _ := tc.GetDetail(ns)
	s.Require().Equal("v1.1.0", detail.ResolvedRef, "PATCH advanced the uninstalled row")

	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, waitTimeout)
	s.exposedDir(env, "tool", "tool v1.1.0")
}
