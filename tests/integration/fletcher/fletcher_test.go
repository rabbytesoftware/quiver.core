//go:build integration

package fletcher_test

import (
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

const (
	toolFixture     = "acme/tool"
	nestedFixture   = "acme/vtool"
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
	forge := s.newForge()
	env := s.NewEnv(kit.WithFletcher(forge.Lookup))
	tc := env.TypedClient(s.T())
	v1 := kit.NSFor(toolFixture, "v1.0.0")
	v2 := kit.NSFor(toolFixture, "v1.1.0")

	s.Require().Equal(http.StatusCreated, tc.Add(v1))
	detail, status := tc.GetDetail(v1)
	s.Require().Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowOriginInferred), detail.Origin)
	s.Require().NotNil(detail.Inference)
	s.Equal("high", detail.Inference.Confidence)
	arrows, listStatus := tc.List()
	s.Require().Equal(http.StatusOK, listStatus)
	s.Require().Len(arrows, 1)
	s.Equal(string(domain.ArrowOriginInferred), arrows[0].Origin)
	s.Equal("high", arrows[0].Confidence)

	s.Require().Equal(http.StatusAccepted, tc.Install(v1, nil))
	env.WaitForState(s.T(), v1, domain.ArrowStateReady, waitTimeout)
	oldWorkdir := s.exposedWorkdir(env, "tool", "tool v1.0.0")

	s.Require().Equal(http.StatusOK, tc.Update(v1, map[string]any{"UpgradeRef": true}))
	env.WaitForState(s.T(), v2, domain.ArrowStateReady, waitTimeout)
	newWorkdir := s.exposedWorkdir(env, "tool", "tool v1.1.0")
	s.Equal("tool@v1.0.0", filepath.Base(oldWorkdir))
	s.Equal("tool@v1.1.0", filepath.Base(newWorkdir))
	s.NoDirExists(oldWorkdir)

	s.Require().Equal(http.StatusAccepted, tc.Uninstall(v2, nil))
	env.WaitForState(s.T(), v2, domain.ArrowStateAbsent, waitTimeout)
	_, err := os.Lstat(s.binLink(env, "tool"))
	s.ErrorIs(err, fs.ErrNotExist)
}

func (s *FletcherSuite) TestFletcher_RuntimeUpdateReinstallsTheNewRef() {
	forge := s.newForge()
	env := s.NewEnv(kit.WithFletcher(forge.Lookup))
	tc := env.TypedClient(s.T())
	v1 := kit.NSFor(toolFixture, "v1.0.0")
	v2 := kit.NSFor(toolFixture, "v1.1.0")

	s.Require().Equal(http.StatusCreated, tc.Add(v1))
	s.Require().Equal(http.StatusAccepted, tc.Install(v1, nil))
	env.WaitForState(s.T(), v1, domain.ArrowStateReady, waitTimeout)
	s.exposedWorkdir(env, "tool", "tool v1.0.0")

	s.Require().Equal(http.StatusAccepted, tc.Execute(v1, "update", nil))
	env.WaitForState(s.T(), v2, domain.ArrowStateReady, waitTimeout)
	newWorkdir := s.exposedWorkdir(env, "tool", "tool v1.1.0")
	s.Equal("tool@v1.1.0", filepath.Base(newWorkdir))

	s.Equal(http.StatusUnprocessableEntity, tc.Execute(v2, "update", nil))
}

func (s *FletcherSuite) TestFletcher_RuntimeUpdateReplacesAVersionedArchiveDirectory() {
	forge := s.newForge()
	env := s.NewEnv(kit.WithFletcher(forge.Lookup))
	tc := env.TypedClient(s.T())
	v1 := kit.NSFor(nestedFixture, "v1.0.0")
	v2 := kit.NSFor(nestedFixture, "v1.1.0")

	s.Require().Equal(http.StatusCreated, tc.Add(v1))
	s.Require().Equal(http.StatusAccepted, tc.Install(v1, nil))
	env.WaitForState(s.T(), v1, domain.ArrowStateReady, waitTimeout)
	s.exposedWorkdir(env, "vtool", "vtool v1.0.0")

	s.Require().Equal(http.StatusAccepted, tc.Execute(v1, "update", nil))
	env.WaitForState(s.T(), v2, domain.ArrowStateReady, waitTimeout)
	exposedDir := s.exposedWorkdir(env, "vtool", "vtool v1.1.0")
	workdir := filepath.Dir(exposedDir)
	s.Equal("vtool@v1.1.0", filepath.Base(workdir))
	entries, err := os.ReadDir(workdir)
	s.Require().NoError(err)
	for _, entry := range entries {
		s.NotContains(entry.Name(), "v1.0.0")
	}
}

func (s *FletcherSuite) TestFletcher_LowConfidenceNeedsConfirmation() {
	forge := s.newForge()
	env := s.NewEnv(kit.WithFletcher(forge.Lookup))
	client := env.Client(s.T())
	ns := kit.NSFor(widgetFixture, "v1.0.0")

	unconfirmed := client.Add(ns)
	defer unconfirmed.Body.Close()
	s.Equal(http.StatusConflict, unconfirmed.StatusCode)
	s.Contains(readBody(s.T(), unconfirmed), "confirmation required")

	confirmed := client.AddConfirmed(ns)
	defer confirmed.Body.Close()
	s.Equal(http.StatusCreated, confirmed.StatusCode)

	detail, detailStatus := env.TypedClient(s.T()).GetDetail(ns)
	s.Require().Equal(http.StatusOK, detailStatus)
	s.Require().NotNil(detail.Inference)
	s.Equal("low", detail.Inference.Confidence)
}

func (s *FletcherSuite) TestFletcher_DisabledLeavesManifestlessRepoNotFound() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusNotFound, tc.Add(kit.NSFor(toolFixture, "v1.0.0")))
}

func (s *FletcherSuite) TestFletcher_DeclaredArrowNeverAsksTheForge() {
	forge := s.newForge()
	env := s.NewEnv(kit.WithFletcher(forge.Lookup))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(declaredFixture, "v1")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	detail, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowOriginDeclared), detail.Origin)
	s.Nil(detail.Inference)
	s.Zero(forge.Calls())
}

func (s *FletcherSuite) TestFletcher_PreviewRawCataloguesNothing() {
	forge := s.newForge()
	env := s.NewEnv(kit.WithFletcher(forge.Lookup))
	ns := kit.NSFor(toolFixture, "v1.0.0")

	resp := env.Client(s.T()).Preview(ns, "raw")
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode)
	body := readBody(s.T(), resp)
	s.Contains(body, "```arrow")
	s.Contains(body, "generator")

	arrows, listStatus := env.TypedClient(s.T()).List()
	s.Require().Equal(http.StatusOK, listStatus)
	s.Empty(arrows)
}

func (s *FletcherSuite) newForge() kit.FakeForge {
	return kit.NewFakeForge(s.T(), map[string]kit.ForgeRepo{
		"quiver.test/" + toolFixture: {
			Description: "A tool for integration tests",
			Readme:      "# tool\n\nA tool for integration tests.\n",
			Binary:      "tool",
			Asset:       "tool_{tag}_{os}_{arch}.tar.gz",
			Tags:        []string{"v1.0.0", "v1.1.0"},
		},
		"quiver.test/" + nestedFixture: {
			Description: "A tool whose archives nest the binary in a versioned directory",
			Readme:      "# vtool\n",
			Binary:      "vtool",
			Asset:       "vtool_{tag}_{os}_{arch}.tar.gz",
			Dir:         "vtool-{tag}-{os}-{arch}",
			Tags:        []string{"v1.0.0", "v1.1.0"},
		},
		"quiver.test/" + widgetFixture: {
			Description: "A widget for integration tests",
			Readme:      "# widget\n",
			Binary:      "gadget-cli",
			Asset:       "gadget-cli_{os}_{arch}.tar.gz",
			Tags:        []string{"v1.0.0"},
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

func (s *FletcherSuite) exposedWorkdir(
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
			s.T().Skip("windows exposes a .cmd shim, not an executable symlink")
		}
		out, err := exec.Command(link).Output()
		s.Require().NoError(err)
		s.Equal(want+"\n", string(out))
	})
	return filepath.Dir(target)
}

func readBody(
	t *testing.T,
	resp *http.Response,
) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}
