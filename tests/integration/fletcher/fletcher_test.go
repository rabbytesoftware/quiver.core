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

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

const (
	toolFixture     = "acme/tool"
	widgetFixture   = "acme/widget"
	declaredFixture = "quiver-test/tool-a"
	waitTimeout     = 120 * time.Second
)

func TestMain(m *testing.M) { kit.Main(m) }

type FletcherSuite struct{ kit.IntegrationSuite }

func TestFletcherIntegration(t *testing.T) {
	suite.Run(t, new(FletcherSuite))
}

func (s *FletcherSuite) TestFletcher_InferredArrowLifecycle() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	v1 := kit.NSFor(toolFixture, "v1.0.0")
	v2 := kit.NSFor(toolFixture, "v1.1.0")

	s.Require().Equal(http.StatusCreated, tc.Add(v1))
	s.Require().Equal(http.StatusAccepted, tc.Install(v1, nil))
	env.WaitForState(s.T(), v1, domain.ArrowStateReady, waitTimeout)

	detail, status := tc.GetDetail(v1)
	s.Require().Equal(http.StatusOK, status)
	s.Require().Equal(string(domain.ArrowOriginInferred), detail.Origin)
	s.Require().NotNil(detail.Inference)
	s.Require().Equal("high", detail.Inference.Confidence)
	arrows, listStatus := tc.List()
	s.Require().Equal(http.StatusOK, listStatus)
	s.Require().Len(arrows, 1)
	s.Require().Equal(string(domain.ArrowOriginInferred), arrows[0].Origin)
	s.Require().Equal("high", arrows[0].Confidence)

	kit.WaitForDetail(s.T(), tc, v1, "recommended_ref v1.1.0", waitTimeout,
		func(d dto.ArrowDetailDTO, _ int) bool { return d.RecommendedRef == "v1.1.0" })
	oldWorkdir := filepath.Dir(s.exposedDir(env, "tool", "tool v1.0.0"))

	s.Require().Equal(http.StatusOK, tc.Update(v1, map[string]any{"UpgradeRef": true}))
	env.WaitForState(s.T(), v2, domain.ArrowStateReady, waitTimeout)
	newWorkdir := filepath.Dir(s.exposedDir(env, "tool", "tool v1.1.0"))
	s.Require().Equal("tool@v1.0.0", filepath.Base(oldWorkdir))
	s.Require().Equal("tool@v1.1.0", filepath.Base(newWorkdir))
	s.Require().NoDirExists(oldWorkdir)

	s.Require().Equal(http.StatusAccepted, tc.Uninstall(v2, nil))
	env.WaitForState(s.T(), v2, domain.ArrowStateAbsent, waitTimeout)
	_, err := os.Lstat(s.binLink(env, "tool"))
	s.ErrorIs(err, fs.ErrNotExist)
}

func (s *FletcherSuite) TestFletcher_RuntimeUpdateReinstallsTheNewRef() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	v1 := kit.NSFor(toolFixture, "v1.0.0")
	v2 := kit.NSFor(toolFixture, "v1.1.0")

	s.Require().Equal(http.StatusCreated, tc.Add(v1))
	s.Require().Equal(http.StatusAccepted, tc.Install(v1, nil))
	env.WaitForState(s.T(), v1, domain.ArrowStateReady, waitTimeout)
	s.exposedDir(env, "tool", "tool v1.0.0")

	s.Require().Equal(http.StatusAccepted, tc.Execute(v1, "update", nil))
	env.WaitForState(s.T(), v2, domain.ArrowStateReady, waitTimeout)
	newWorkdir := filepath.Dir(s.exposedDir(env, "tool", "tool v1.1.0"))
	s.Equal("tool@v1.1.0", filepath.Base(newWorkdir))

	s.Equal(http.StatusUnprocessableEntity, tc.Execute(v2, "update", nil))
}

func (s *FletcherSuite) TestFletcher_LowConfidenceIsAMissingManifest() {
	host := s.newHost()
	env := s.NewEnv(kit.WithFletcher(host.Lookup))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(widgetFixture, "v1.0.0")

	s.Equal(http.StatusNotFound, tc.Add(ns))
	s.Require().Positive(host.Calls())
	callsAfterAdd := host.Calls()
	_, detailStatus := tc.GetDetail(ns)
	s.Equal(http.StatusNotFound, detailStatus)
	s.Equal(callsAfterAdd, host.Calls())
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
