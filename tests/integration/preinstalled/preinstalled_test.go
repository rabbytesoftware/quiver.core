//go:build integration

package preinstalled_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type PreinstalledSuite struct{ kit.IntegrationSuite }

func TestPreinstalledIntegration(t *testing.T) {
	suite.Run(t, new(PreinstalledSuite))
}

// fixtureKey is the key the kit's fixture resolver files this suite's arrow
// under: the bare namespace with the "quiver.test/" prefix stripped. It is
// registered per test rather than read from testdata/arrows/ because each run
// needs its own marker path baked into the manifest — a fixed path on disk
// would make two concurrent runs of this suite answer each other's probes.
const fixtureKey = "quiver-test/preinstalled-detect"

// manifestWithMarker builds an arrow that opts into Add-time detection. Its
// preinstalled step is a real command against a real path on disk, reached
// through a manifest-declared variable, so the probe exercises the same
// variable expansion and the same step handler an install lifecycle does.
//
// The arrow is otherwise ordinary: it declares install, execute and uninstall
// lifecycles, because a detected arrow has to be a usable one — Ready without
// an install still has to mean Ready.
func manifestWithMarker(markerPath string) []byte {
	return []byte(fmt.Sprintf(`schema: "arrow@v0"
metadata:
  name: quiver-test.preinstalled-detect
  description: Detects an install Quiver did not perform
variables:
  - name: MARKER_PATH
    description: Path the preinstalled check looks for
    default: %q
targets:
  "*":
    lifecycle:
      preinstalled:
        - type: run
          command: test -f ${MARKER_PATH}
          title: Detect existing install
          timeout: 10s
          exit_on_failure: true
      install:
        - type: run
          command: echo installed
          title: Install
          timeout: 10s
          exit_on_failure: true
      execute:
        - type: run
          command: echo executed
          title: Execute
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`, markerPath))
}

// registerFixture publishes the marker-aware manifest as this suite's fixture
// repo for the duration of one test.
func (s *PreinstalledSuite) registerFixture(markerPath string) string {
	s.T().Helper()

	storer := kit.BuildUpgradeRepo(s.T(), manifestWithMarker(markerPath))
	s.Repos.Set(fixtureKey, storer)
	s.T().Cleanup(func() { s.Repos.Delete(fixtureKey) })

	return kit.NSFor(fixtureKey, "v1")
}

// writeMarker creates the file the preinstalled check looks for, standing in
// for software installed on this machine by something other than Quiver.
func (s *PreinstalledSuite) writeMarker(path string) {
	s.T().Helper()
	require.NoError(s.T(), os.WriteFile(path, []byte("installed elsewhere"), 0o600))
}

func (s *PreinstalledSuite) getDetail(tc *kit.TypedClient, ns string) dto.ArrowDetailDTO {
	s.T().Helper()
	detail, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status, "GetDetail(%s) must return 200", ns)
	return detail
}

// TestPreinstalled_Detected_ReadableAsReadyImmediately is this feature's whole
// promise, against the real daemon: an arrow whose preinstalled check finds
// something is readable as user-installed and Ready the moment Add returns.
//
// Every assertion here is a single read with no polling and no
// require.Eventually, on purpose. A retry loop would pass just as happily on an
// implementation that reached Ready a moment later through some asynchronous
// reaction, which is exactly the design this test exists to rule out. If there
// were an async window, the first read would land inside it.
func (s *PreinstalledSuite) TestPreinstalled_Detected_ReadableAsReadyImmediately() {
	marker := filepath.Join(s.T().TempDir(), "already-installed")
	s.writeMarker(marker)
	ns := s.registerFixture(marker)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	require.Equal(s.T(), http.StatusCreated, tc.Add(ns))

	detail := s.getDetail(tc, ns)
	require.True(s.T(), detail.UserInstalled,
		"a detected arrow is in the catalog as user-installed")
	require.Equal(s.T(), string(domain.ArrowStateReady), detail.State,
		"the very first read after Add must already be Ready — no window, not a short one")
	require.Nil(s.T(), detail.ActiveRun, "nothing ran: the arrow was already there")
	require.Nil(s.T(), detail.LastReturn, "nothing ran, so there is no return to report")
	require.Empty(s.T(), detail.InstalledAt,
		"Quiver never installed this arrow, so it carries no install stamp")
}

// TestPreinstalled_Detected_ReadyIsUsable: Ready reached by detection has to be
// the same Ready an install reaches, not a label. The proof is that the arrow
// can be executed straight away, with no install first.
func (s *PreinstalledSuite) TestPreinstalled_Detected_ReadyIsUsable() {
	marker := filepath.Join(s.T().TempDir(), "already-installed")
	s.writeMarker(marker)
	ns := s.registerFixture(marker)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	require.Equal(s.T(), http.StatusCreated, tc.Add(ns))
	require.Equal(s.T(), string(domain.ArrowStateReady), s.getDetail(tc, ns).State)

	require.Equal(s.T(), http.StatusAccepted, tc.Execute(ns, "execute", nil),
		"a detected arrow accepts an execution without ever having been installed")
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 60*time.Second)

	detail := s.getDetail(tc, ns)
	require.NotNil(s.T(), detail.LastReturn, "the execution really ran")
	require.Equal(s.T(), "success", detail.LastReturn.Outcome)
}

// TestPreinstalled_NotDetected_BehavesLikeAnyOtherArrow is the regression half:
// the manifest declares the block, the probe genuinely runs against a marker
// that is not there, and the arrow must land exactly where an ordinary add
// leaves it — Absent, installable, unchanged.
func (s *PreinstalledSuite) TestPreinstalled_NotDetected_BehavesLikeAnyOtherArrow() {
	missing := filepath.Join(s.T().TempDir(), "never-created")
	ns := s.registerFixture(missing)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	require.Equal(s.T(), http.StatusCreated, tc.Add(ns))

	detail := s.getDetail(tc, ns)
	require.True(s.T(), detail.UserInstalled)
	require.Equal(s.T(), string(domain.ArrowStateAbsent), detail.State,
		"a check that finds nothing leaves the arrow exactly where Add always left it")

	require.Equal(s.T(), http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	installed := s.getDetail(tc, ns)
	require.NotEmpty(s.T(), installed.InstalledAt,
		"the ordinary install path still stamps the arrow")
}

// TestPreinstalled_NoBlockDeclared_UnchangedForEveryOtherArrow guards the thing
// it would be worst to break: every arrow in the system that does not opt in
// still adds and installs exactly as before. tool-a is an ordinary testdata
// fixture with no preinstalled key anywhere in its manifest.
func (s *PreinstalledSuite) TestPreinstalled_NoBlockDeclared_UnchangedForEveryOtherArrow() {
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	require.Equal(s.T(), http.StatusCreated, tc.Add(ns))

	detail := s.getDetail(tc, ns)
	require.True(s.T(), detail.UserInstalled)
	require.Equal(s.T(), string(domain.ArrowStateAbsent), detail.State)
	require.Empty(s.T(), detail.InstalledAt)

	require.Equal(s.T(), http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)
}

// TestPreinstalled_AddTwice_StaysReady is the shape quiver.desktop will
// actually use this in: it announces itself on every boot, so the second and
// every later Add has to be a harmless no-op rather than a re-probe that could
// disturb a runtime that has since moved on.
func (s *PreinstalledSuite) TestPreinstalled_AddTwice_StaysReady() {
	marker := filepath.Join(s.T().TempDir(), "already-installed")
	s.writeMarker(marker)
	ns := s.registerFixture(marker)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	require.Equal(s.T(), http.StatusCreated, tc.Add(ns))
	require.Equal(s.T(), string(domain.ArrowStateReady), s.getDetail(tc, ns).State)

	// The marker is gone by the second boot; a re-probe would now find nothing.
	// The arrow must stay Ready regardless, because the second Add must not
	// probe at all.
	require.NoError(s.T(), os.Remove(marker))

	require.Equal(s.T(), http.StatusCreated, tc.Add(ns))

	detail := s.getDetail(tc, ns)
	require.True(s.T(), detail.UserInstalled)
	require.Equal(s.T(), string(domain.ArrowStateReady), detail.State,
		"a repeated announcement must not re-probe, and must not disturb the runtime")
}

// TestPreinstalled_Detected_SurvivesRestart: the detection is written to the
// runtime aggregate, not held in memory, so a daemon restart on the same home
// still reads Ready without re-probing. Mirrors the env1/env2 pairing the
// crash and self-update suites use.
func (s *PreinstalledSuite) TestPreinstalled_Detected_SurvivesRestart() {
	marker := filepath.Join(s.T().TempDir(), "already-installed")
	s.writeMarker(marker)
	ns := s.registerFixture(marker)

	env1 := s.NewEnv()
	tc1 := env1.TypedClient(s.T())
	require.Equal(s.T(), http.StatusCreated, tc1.Add(ns))
	require.Equal(s.T(), string(domain.ArrowStateReady), s.getDetail(tc1, ns).State)

	env1.Close()

	require.NoError(s.T(), os.Remove(marker), "nothing may re-probe after the restart")

	env2 := s.NewEnvWithHome(env1.Home)
	tc2 := env2.TypedClient(s.T())

	detail := s.getDetail(tc2, ns)
	require.True(s.T(), detail.UserInstalled)
	require.Equal(s.T(), string(domain.ArrowStateReady), detail.State)
}
