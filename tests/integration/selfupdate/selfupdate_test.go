//go:build integration

package selfupdate_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type SelfUpdateSuite struct{ kit.IntegrationSuite }

func TestSelfUpdateIntegration(t *testing.T) {
	suite.Run(t, new(SelfUpdateSuite))
}

// selfNamespace is the exact namespace repositories/container.go's
// claimSuccession matches on (strings.HasPrefix(rt.Ref.String(),
// string(selfarrow.Namespace)+"@")). The trigger only ever fires for a
// runtime whose ref carries this literal prefix, so the fixture standing in
// for quiver.core's own manifest has to be registered under it — the usual
// "quiver.test/..." fixture convention would never be recognized by that
// check.
const selfNamespace = "github.com/rabbytesoftware/quiver.core"

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
// It does not exercise the literal OS-level exec handover (impossible inside
// this test binary — syscall.Exec would replace the test binary's own
// process image); it exercises everything up to and through it, using the
// same env1->env2 pairing this codebase's crash-recovery tests already use
// (tests/integration/crash/crash_test.go) to simulate "a process restarted
// with no in-memory state" — exactly what a real exec handover leaves the new
// binary with.
func (s *SelfUpdateSuite) TestSelfUpdate_SupervisedProcessSurvives_AcrossRestart() {
	selfNS := selfNamespace + "@v1"

	v1Content := kit.ReadFixture(s.T(), "self-update/v1/arrow.yaml")
	v2Content := kit.ReadFixture(s.T(), "self-update/v2/arrow.yaml")
	storer := kit.BuildUpgradeRepo(s.T(), v1Content)
	kit.AddV2ToRepo(s.T(), storer, v2Content)
	s.Repos.Set(selfNamespace, storer)
	s.T().Cleanup(func() { s.Repos.Delete(selfNamespace) })

	trig := selfupdate.NewTrigger(nil)
	env1 := s.NewEnvWithSelfUpdateTrigger(trig)
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

	require.Eventually(s.T(), trig.Fired, 10*time.Second, 50*time.Millisecond,
		"the OnRuntimeEnded -> trigger wiring must fire through the real app-layer DI, not just Task 1.4's own unit test")

	env1.CloseWithoutKilling() // the fixture's OS process survives — mirrors what a real exec handover leaves behind

	env2 := s.NewEnvWithHome(env1.Home)
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
