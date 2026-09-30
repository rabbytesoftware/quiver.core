//go:build integration

package fletcher_test

import (
	"io/fs"
	"net/http"
	"os"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func (s *FletcherSuite) downloadsPerInstall(
	env *kit.Env,
	host kit.FakeHost,
) int64 {
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(plainFixture, "v1.0.0")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, waitTimeout)
	return host.Downloads()
}

func (s *FletcherSuite) TestFletcher_RepublishedAssetIsRefreshedAndInstalledOnce() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	perInstall := s.downloadsPerInstall(env, host)
	ns := kit.NSFor(toolFixture, "v1.0.0")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	host.Republish(s.T(), domain.Namespace(ns), "v1.0.0")
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))

	env.WaitForState(s.T(), ns, domain.ArrowStateReady, waitTimeout)
	s.Equal(3*perInstall, host.Downloads())
	_, err := os.Lstat(s.binLink(env, "tool"))
	s.NoError(err)
}

func (s *FletcherSuite) TestFletcher_CorruptAssetFailsWithoutRetrying() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	perInstall := s.downloadsPerInstall(env, host)
	ns := kit.NSFor(toolFixture, "v1.0.0")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	host.Corrupt(domain.Namespace(ns), "v1.0.0")
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))

	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, waitTimeout)
	s.Equal(2*perInstall, host.Downloads())
	_, err := os.Lstat(s.binLink(env, "tool"))
	s.ErrorIs(err, fs.ErrNotExist)
}
