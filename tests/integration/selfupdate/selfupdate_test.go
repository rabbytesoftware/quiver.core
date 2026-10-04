//go:build integration

package selfupdate_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type SelfUpdateSuite struct{ kit.IntegrationSuite }

func TestSelfUpdateIntegration(t *testing.T) {
	suite.Run(t, new(SelfUpdateSuite))
}

// selfNamespace is quiver.core's own namespace (metadata.GetSelfNamespaces()):
// the lifecycle and the boot registration treat the row under it specially, so
// the fixture standing in for quiver.core's own manifest has to be registered
// under it.
const selfNamespace = "github.com/rabbytesoftware/quiver.core"

// noVersionCheck fails every live snapshot, so no version check can ever
// reach a verdict.
type noVersionCheck struct{ manifold.Manifold }

func (noVersionCheck) FreshSnapshot(context.Context, domain.Namespace) (domain.RefSnapshot, error) {
	return domain.RefSnapshot{}, errors.New("version checks are disabled in this test")
}

// getDetail fetches arrow detail and requires HTTP 200. Mirrors the identical
// small helper every other integration suite defines locally (see
// tests/integration/versioning/versioning_test.go).
func (s *SelfUpdateSuite) getDetail(tc *kit.TypedClient, ns string) dto.ArrowDetailDTO {
	s.T().Helper()
	detail, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status, "GetDetail(%s) must return 200", ns)
	return detail
}

// TestSelfUpdate_SupervisedProcessSurvives_AcrossRestart is the concrete,
// automatable proof behind this feature's core promise: a process the daemon
// is actively supervising is never killed by the daemon's own self-update.
// It does not start a real replacement daemon (that is the end-to-end proof in
// tests/integration/selfupdate/swap_test.go); it exercises the restart the swap
// leaves the new binary with, using the same env1->env2 pairing this
// codebase's crash-recovery tests already use (tests/integration/crash/crash_test.go)
// to simulate "a process restarted with no in-memory state".
func (s *SelfUpdateSuite) TestSelfUpdate_SupervisedProcessSurvives_AcrossRestart() {
	// The self-arrow follows a selector (here the constraint v*, standing in
	// for the release channel the fixture's v1/v2 tags do not classify into);
	// an update only runs when that selector has moved ahead of what is
	// installed, so v2 is published after the self-arrow is installed at v1.
	selfNS := selfNamespace + "@v*"

	v1Content := kit.ReadFixture(s.T(), "self-update/v1/arrow.yaml")
	v2Content := kit.ReadFixture(s.T(), "self-update/v2/arrow.yaml")
	storer := kit.BuildUpgradeRepo(s.T(), v1Content)
	s.Repos.Set(selfNamespace, storer)
	s.T().Cleanup(func() { s.Repos.Delete(selfNamespace) })

	env1 := s.NewEnv()
	tc1 := env1.TypedClient(s.T())

	// --- the supervised process that must survive Core's own self-update ---
	fixtureNS := kit.NSFor("quiver-test/self-update-fixture", "v1")
	require.Equal(s.T(), http.StatusCreated, tc1.Add(fixtureNS))
	require.Equal(s.T(), http.StatusAccepted, tc1.Install(fixtureNS, nil))
	env1.WaitForState(s.T(), fixtureNS, domain.ArrowStateReady, 120*time.Second)
	require.Equal(s.T(), http.StatusAccepted, tc1.Execute(fixtureNS, "execute", nil))
	env1.WaitForState(s.T(), fixtureNS, domain.ArrowStateRunning, 120*time.Second)
	env1.WaitForActivePID(s.T(), fixtureNS, 120*time.Second)

	detail := s.getDetail(tc1, fixtureNS)
	require.NotNil(s.T(), detail.ActiveRun)
	originalPID := detail.ActiveRun.PID
	require.Greater(s.T(), originalPID, 0)
	// Registered as a safety net right away, not deferred until the explicit
	// cleanup near the end of this test: every assertion between here and
	// there sits on the same window a failure earlier in this plan's history
	// has actually hit — env1's own CloseWithoutKilling deliberately never
	// tears down its wizard (see its doc comment), so env1 stays valid to
	// kill through for the rest of the process's life, regardless of where
	// or whether the test itself gets that far. The explicit
	// env2.KillDetachedProcess call further down still runs first in the
	// normal, non-failing path — t.Cleanup only takes over when something
	// upstream of it fails or panics.
	s.T().Cleanup(func() { env1.KillDetachedProcess(s.T(), originalPID) })

	// --- self-arrow: registered, then bootstrapped to Ready. Its manifest
	// (mirroring Task 1.1's real one) defines only an update lifecycle — no
	// install — so BeginInstall's own dependency-resolution step is the only
	// thing that runs; "already installed by definition" made concrete. ---
	require.Equal(s.T(), http.StatusCreated, tc1.Add(selfNS))
	// ResolveVariables (internal/app/repositories/runtime/internal/assembler/internal/variables.go)
	// requires every arrow-level declared variable with no default to be
	// supplied on ANY execution of the arrow, not just the lifecycle method
	// that references it — so QUIVER_RELEASE_ASSET_URL/CHECKSUM must be
	// supplied here too, even though Install never reads them.
	bootstrapVars := map[string]string{
		"QUIVER_RELEASE_ASSET_URL": "unused-for-install",
		"QUIVER_RELEASE_CHECKSUM":  "unused-for-install",
	}
	require.Equal(s.T(), http.StatusAccepted, tc1.Install(selfNS, bootstrapVars))
	env1.WaitForState(s.T(), selfNS, domain.ArrowStateReady, 120*time.Second)

	v2Commit := kit.AddV2ToRepo(s.T(), storer, v2Content)

	// --- fixture HTTP server standing in for the resolved release asset,
	// exercising Task 1.5's checksum verification rather than bypassing it ---
	payload := []byte("fake quiver.core release binary contents")
	sum := sha256.Sum256(payload)
	checksum := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	require.Equal(s.T(), http.StatusAccepted, tc1.Execute(selfNS, "update", map[string]string{
		"QUIVER_RELEASE_ASSET_URL": srv.URL,
		"QUIVER_RELEASE_CHECKSUM":  checksum,
	}))

	kit.WaitForLastReturn(s.T(), tc1, selfNS, 1, 60*time.Second)

	env1.WaitForState(s.T(), selfNS, domain.ArrowStateReady, 60*time.Second)

	env1.CloseWithoutKilling() // the fixture's OS process survives — mirrors what a real swap leaves behind

	// The relaunched binary is the v2 build: it adopts its own new state on
	// the self-arrow's identity, which is what settles that row. Its version
	// checks can never answer, so only that adoption can clear the badge.
	env2 := kit.BuildEnv(s.T(), s.Repos, s.CollectionRepos, env1.Home,
		kit.WithBuild("v2", v2Commit, "v*"),
		kit.WithManifoldWrapper(func(inner manifold.Manifold) manifold.Manifold {
			return noVersionCheck{Manifold: inner}
		}))
	tc2 := env2.TypedClient(s.T())

	finalFixture := s.getDetail(tc2, fixtureNS)
	require.Equal(s.T(), string(domain.ArrowStateDetached), finalFixture.State,
		"per the accepted design: other arrows land Detached (one-click reattach), not killed and not silently Running")
	// RecordDetached deliberately clears Execution (see
	// internal/app/repositories/runtime/internal/commands/record_detached.go),
	// so ActiveRun is nil once detached — there is no read-model field left to
	// compare the PID against. The OS process itself is the only thing left
	// to check, which is also the only thing this test actually claims to
	// prove: the PID captured before the restart is still alive after it.
	require.True(s.T(), env2.ProcessAlive(originalPID), "unchanged PID — the process was never killed")

	finalSelf := s.getDetail(tc2, selfNS)
	require.Equal(s.T(), string(domain.ArrowStateReady), finalSelf.State,
		"the self-arrow's own record settles back to Ready — it never reaches Running, see this task's header note")

	// Cleanup: the fixture is Detached, not tracked by either env's own
	// Close() (env1's Close was bypassed via CloseWithoutKilling, and env2
	// never spawned the process itself). Going through the API's Stop here
	// would not help: BeginStop reads its target PID from
	// current.Execution.PID, and RecordDetached already cleared Execution to
	// nil (see KillDetachedProcess's own doc comment) — this was confirmed
	// empirically while writing this test: tc2.Stop reports 202 and the arrow
	// reaches Ready, but the sleep 300 process is left running because
	// nothing ever signals it. Kill it directly by the PID captured before
	// the restart, so the test doesn't leak a sleep 300 past completion.
	env2.KillDetachedProcess(s.T(), originalPID)
	require.Eventually(s.T(), func() bool { return !env2.ProcessAlive(originalPID) }, 5*time.Second, 50*time.Millisecond,
		"the fixture process must be gone after cleanup")
}

// libraryRefs returns the refs GET /v0/arrow?user_installed=true lists for
// quiver.core: what the desktop library shows of it.
func (s *SelfUpdateSuite) libraryRefs(tc *kit.TypedClient) []string {
	s.T().Helper()
	items, status := tc.ListUserInstalled()
	s.Require().Equal(http.StatusOK, status)
	var refs []string
	for _, item := range items {
		if item.Namespace != selfNamespace {
			continue
		}
		for _, v := range item.Versions {
			refs = append(refs, v.Ref)
		}
	}
	return refs
}

type build struct{ version, commit, channel string }

// TestSelfUpdate_CoreStaysInTheLibraryAcrossRestarts is the regression for a
// core that updated itself dropping out of the desktop library: each restart
// boots a newer build on the same home, and the library must still list it.
func (s *SelfUpdateSuite) TestSelfUpdate_CoreStaysInTheLibraryAcrossRestarts() {
	testCases := []struct {
		name    string
		first   build
		next    build
		wantRef string
	}{
		{
			name:    "same identity, new version and commit",
			first:   build{"beta-26.5-4", "aaa", "beta"},
			next:    build{"beta-26.5-5", "bbb", "beta"},
			wantRef: "beta",
		},
		{
			name:    "rolling tag, new commit",
			first:   build{"nightly-latest", "aaa", "nightly-latest"},
			next:    build{"nightly-latest", "bbb", "nightly-latest"},
			wantRef: "nightly-latest",
		},
		{
			name:    "different identity",
			first:   build{"stable-26.5.1", "aaa", ""},
			next:    build{"stable-26.5.2", "bbb", "stable"},
			wantRef: "stable",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			home := s.T().TempDir()
			env1 := kit.BuildEnv(s.T(), s.Repos, s.CollectionRepos, home, kit.WithBuild(tc.first.version, tc.first.commit, tc.first.channel))
			s.Require().NotEmpty(s.libraryRefs(env1.TypedClient(s.T())))
			env1.Close()

			env2 := kit.BuildEnv(s.T(), s.Repos, s.CollectionRepos, home, kit.WithBuild(tc.next.version, tc.next.commit, tc.next.channel))
			tc2 := env2.TypedClient(s.T())

			s.Equal([]string{tc.wantRef}, s.libraryRefs(tc2))
			s.True(s.getDetail(tc2, selfNamespace+"@"+tc.wantRef).UserInstalled)
			env2.Close()
		})
	}
}

// TestSelfUpdate_RowLeftOutOfTheLibrary_IsRepairedOnBoot starts from the state
// the bug left behind: a quiver.core row catalogued without the user-installed
// flag, here as another arrow's dependency. The next boot of a released build
// on that identity puts it back in the library.
func (s *SelfUpdateSuite) TestSelfUpdate_RowLeftOutOfTheLibrary_IsRepairedOnBoot() {
	s.Repos.Set(selfNamespace, kit.BuildUpgradeRepo(s.T(), kit.ReadFixture(s.T(), "self-update/v1/arrow.yaml")))
	s.T().Cleanup(func() { s.Repos.Delete(selfNamespace) })
	const dependentKey = "quiver-test/depends-on-core"
	dependentManifest := []byte(`schema: "arrow@v0"
metadata:
  name: quiver-test.depends-on-core
  description: Depends on quiver.core
targets:
  "*":
    tools:
      - ` + selfNamespace + `@v1
    lifecycle:
      install:
        - type: run
          command: echo installed
          title: Install
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`)
	s.Repos.Set(dependentKey, kit.BuildUpgradeRepo(s.T(), dependentManifest))
	s.T().Cleanup(func() { s.Repos.Delete(dependentKey) })

	home := s.T().TempDir()
	env1 := kit.BuildEnv(s.T(), s.Repos, s.CollectionRepos, home)
	tc1 := env1.TypedClient(s.T())
	dependent := kit.NSFor(dependentKey, "v1")
	s.Require().Equal(http.StatusCreated, tc1.Add(dependent))
	s.Require().Equal(http.StatusAccepted, tc1.Install(dependent, nil))
	env1.WaitForState(s.T(), dependent, domain.ArrowStateReady, 120*time.Second)
	s.Require().Empty(s.libraryRefs(tc1), "precondition: the core's row is catalogued outside the library")
	s.Require().False(s.getDetail(tc1, selfNamespace+"@v1").UserInstalled)
	env1.Close()

	env2 := kit.BuildEnv(s.T(), s.Repos, s.CollectionRepos, home, kit.WithBuild("v1", "bbb", ""))
	tc2 := env2.TypedClient(s.T())

	s.Equal([]string{"v1"}, s.libraryRefs(tc2))
	s.True(s.getDetail(tc2, selfNamespace+"@v1").UserInstalled)
}
