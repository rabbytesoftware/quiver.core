//go:build integration

package fletcher_test

import (
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

const (
	toolFixture      = "acme/tool"
	widgetFixture    = "acme/widget"
	declaredFixture  = "quiver-test/tool-a"
	releasingFixture = "acme-releases/tool"
	waitTimeout      = 120 * time.Second
)

func TestMain(m *testing.M) { kit.Main(m) }

type FletcherSuite struct{ kit.IntegrationSuite }

func TestFletcherIntegration(t *testing.T) {
	suite.Run(t, new(FletcherSuite))
}

// A stable-channel row of a repository with no ARROW.md installs the drafted
// manifest, follows a new release in place, and uninstalls cleanly.
func (s *FletcherSuite) TestFletcher_InferredArrowLifecycle() {
	storer := s.releasedFixture(releasingFixture, "v1.0.0")
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(releasingFixture, "stable")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, waitTimeout)

	detail, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status)
	s.Require().Equal(string(domain.ArrowOriginInferred), detail.Origin)
	s.Require().NotNil(detail.Inference)
	s.Require().Equal("high", detail.Inference.Confidence)
	s.Equal("v1.0.0", detail.ResolvedRef)
	arrows, listStatus := tc.List()
	s.Require().Equal(http.StatusOK, listStatus)
	s.Require().Len(arrows, 1)
	s.Require().Equal(string(domain.ArrowOriginInferred), arrows[0].Origin)
	s.Require().Equal("high", arrows[0].Confidence)
	workdir := s.exposedDir(env, "tool", "tool v1.0.0")

	var released string
	s.Repos.Mutate(func() { released = kit.TagHead(s.T(), storer, "v1.1.0") })
	result, checkStatus := tc.CheckAvailable(ns)
	s.Require().Equal(http.StatusOK, checkStatus)
	s.Require().NotNil(result.Available)
	s.Equal(dto.AvailableDTO{Ref: "v1.1.0", Commit: released}, *result.Available)

	s.Require().Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodUpdate, nil))
	kit.WaitForDetail(s.T(), tc, ns, "the row advanced to v1.1.0", waitTimeout,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK &&
				d.ResolvedRef == "v1.1.0" &&
				d.Available == nil &&
				d.State == string(domain.ArrowStateReady)
		})
	s.Equal(workdir, s.exposedDir(env, "tool", "tool v1.1.0"), "an update reinstalls in place")
	s.Equal(http.StatusOK, tc.Execute(ns, domain.MethodUpdate, nil), "nothing is ahead any more")

	s.Require().Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, waitTimeout)
	_, err := os.Lstat(s.binLink(env, "tool"))
	s.ErrorIs(err, fs.ErrNotExist)
}

// Every release of the fixture shares one commit, so only the ref the pin
// names can say which release's assets the drafted manifest installs.
func (s *FletcherSuite) TestFletcher_PinnedReleaseDraftsItsOwnTag() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(toolFixture, "v1.0.0")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, waitTimeout)
	s.exposedDir(env, "tool", "tool v1.0.0")

	s.Equal(http.StatusOK, tc.Execute(ns, domain.MethodUpdate, nil), "a pin has nothing ahead of it")
	s.exposedDir(env, "tool", "tool v1.0.0")
}

func (s *FletcherSuite) TestFletcher_LowConfidenceIsAMissingManifest() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(widgetFixture, "v1.0.0")

	s.Equal(http.StatusNotFound, tc.Add(ns))
	s.Require().Positive(host.Calls())
	_, detailStatus := tc.GetDetail(ns)
	s.Equal(http.StatusNotFound, detailStatus)
	arrows, listStatus := tc.List()
	s.Require().Equal(http.StatusOK, listStatus)
	s.Empty(arrows)
}

func (s *FletcherSuite) TestFletcher_DisabledLeavesManifestlessRepoNotFound() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(toolFixture, "v1.0.0")

	s.Equal(http.StatusNotFound, tc.Add(ns))
	_, detailStatus := tc.GetDetail(ns)
	s.Equal(http.StatusNotFound, detailStatus)
}

func (s *FletcherSuite) TestFletcher_DeclaredArrowNeverAsksTheHost() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(declaredFixture, "v1")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	detail, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowOriginDeclared), detail.Origin)
	s.Nil(detail.Inference)
	s.Zero(host.Calls())
}

func (s *FletcherSuite) TestFletcher_DetailOfAnUncataloguedRepoCataloguesNothing() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())

	detail, status := tc.GetDetail(kit.NSFor(toolFixture, "v1.0.0"))
	s.Require().Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowOriginInferred), detail.Origin)
	s.Require().NotNil(detail.Inference)
	s.Equal("high", detail.Inference.Confidence)

	arrows, listStatus := tc.List()
	s.Require().Equal(http.StatusOK, listStatus)
	s.Empty(arrows)
}

func (s *FletcherSuite) newHost() kit.FakeHost {
	return kit.NewFakeHost(s.T(), map[string]kit.HostRepo{
		"quiver.test/" + toolFixture: {
			Description: "A tool for integration tests",
			Readme:      "# tool\n\nA tool for integration tests.\n",
			Binary:      "tool",
			Asset:       "tool_{tag}_{os}_{arch}.tar.gz",
			Tags:        []string{"v1.0.0", "v1.1.0"},
		},
		"quiver.test/" + releasingFixture: {
			Description: "A tool for integration tests",
			Readme:      "# tool\n\nA tool for integration tests.\n",
			Binary:      "tool",
			Asset:       "tool_{tag}_{os}_{arch}.tar.gz",
			Tags:        []string{"v1.0.0", "v1.1.0"},
		},
		"quiver.test/" + widgetFixture: {
			Description: "A widget for integration tests",
			Readme:      "# widget\n",
			Binary:      "gadget-cli",
			Asset:       "gadget-cli_{os}_{arch}.tar.gz",
			Tags:        []string{"v1.0.0"},
		},
		"quiver.test/" + declaredFixture: {
			Description: "A repo that ships its own ARROW.md",
			Readme:      "# tool-a\n",
			Binary:      "tool-a",
			Asset:       "tool-a_{tag}_{os}_{arch}.tar.gz",
			Tags:        []string{"v1"},
		},
	})
}

// releasedFixture registers a manifestless repository owned by one test, since
// the test cuts a release on it.
func (s *FletcherSuite) releasedFixture(
	key string,
	tag string,
) *memory.Storage {
	storer := kit.BuildManifestlessRepo(s.T(), tag)
	s.Repos.Set(key, storer)
	s.T().Cleanup(func() { s.Repos.Delete(key) })
	return storer
}

func (s *FletcherSuite) binLink(
	env *kit.Env,
	name string,
) string {
	bin, err := paths.BinAt(env.Home)
	s.Require().NoError(err)
	return filepath.Join(bin, name)
}

func (s *FletcherSuite) exposedDir(
	env *kit.Env,
	name string,
	want string,
) string {
	link := s.binLink(env, name)
	info, err := os.Lstat(link)
	s.Require().NoError(err)
	s.Require().NotZero(info.Mode() & fs.ModeSymlink)
	target, err := filepath.EvalSymlinks(link)
	s.Require().NoError(err)
	s.Run("executes "+want, func() {
		if runtime.GOOS == "windows" {
			s.T().Skip("windows exposes the command folder on the user Path, not an executable symlink")
		}
		out, err := exec.Command(link).Output()
		s.Require().NoError(err)
		s.Equal(want+"\n", string(out))
	})
	return filepath.Dir(target)
}
