//go:build integration

// Package advance_test proves the request-keyed versioning model end to end:
// a row is keyed by the selector the user follows, an update moves that same
// row in place, and a bracket commits it only when the target held still.
//
// Tests never read GET /arrow/:ns of a row before moving its repo: the first
// read claims the passive version check, and a check that read the repo
// before a move may land after the explicit one that read it afterwards.
package advance_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type AdvanceSuite struct{ kit.IntegrationSuite }

func TestAdvanceIntegration(t *testing.T) {
	suite.Run(t, new(AdvanceSuite))
}

func (s *AdvanceSuite) rollingFixture() fixture {
	return s.newFixture("rolling-tool", "nightly", kit.ReadFixture(s.T(), "rolling-tool/arrow.yaml"))
}

func (s *AdvanceSuite) stableManifest() []byte {
	return kit.ReadFixture(s.T(), "stable-tool/arrow.yaml")
}

func (s *AdvanceSuite) TestAdvance_RollingTag_ForceMoved_UpdatesInPlace() {
	f := s.rollingFixture()
	installed := s.tagCommit(f, "nightly")
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("nightly")

	s.install(env, tc, ns)
	s.Nil(s.checkAvailable(tc, ns), "a freshly installed rolling tag has nothing ahead of it")

	moved := s.moveTag(f, "nightly")
	s.Require().NotEqual(installed, moved)
	poller := s.pollDetail(env, ns)

	available := s.checkAvailable(tc, ns)
	s.Require().NotNil(available, "a force-moved rolling tag must be detected")
	s.Equal("nightly", available.Ref)
	s.Equal(moved, available.Commit)

	outdated := kit.WaitForDetail(s.T(), tc, ns, "the moved commit recorded as available", wait,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.Available != nil && d.Available.Commit == moved
		},
	)
	s.Equal("channel", outdated.SelectorKind)
	s.Equal("nightly", outdated.ResolvedRef)
	s.Equal(installed, outdated.InstalledCommit, "a check never moves what is installed")
	s.True(outdated.Outdated)

	s.update(tc, ns)
	advanced := s.waitAdvanced(tc, ns, moved)

	polls, bad := poller.stop()
	s.Positive(polls)
	s.Empty(bad, "GET /arrow/%s must answer 200 throughout the check and the update", ns)
	s.Equal(ns, advanced.Namespace, "an update keeps the identity")
	s.Equal("nightly", advanced.ResolvedRef)
	s.False(advanced.Outdated)

	marker, ok := s.workFile(env, ns, installMarker)
	s.True(ok, "an update overwrites in place: the workdir the install wrote survives")
	s.Equal("installed\n", marker)
	s.Equal([]string{"nightly"}, s.updateRuns(env, ns))
}

func (s *AdvanceSuite) TestAdvance_StableChannel_KeepsIdentityAcrossReleases() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.2.0", manifest)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("stable")

	s.install(env, tc, ns)
	s.Nil(s.checkAvailable(tc, ns))

	v130 := s.publish(f, "v1.3.0", manifest)
	poller := s.pollDetail(env, ns)

	available := s.checkAvailable(tc, ns)
	s.Require().NotNil(available)
	s.Equal(dto.AvailableDTO{Ref: "v1.3.0", Commit: v130}, *available)

	s.update(tc, ns)
	advanced := s.waitAdvanced(tc, ns, v130)

	polls, bad := poller.stop()
	s.Positive(polls)
	s.Empty(bad, "%s must stay resolvable while it moves to v1.3.0", ns)
	s.Equal(ns, advanced.Namespace)
	s.Equal("channel", advanced.SelectorKind)
	s.Equal("v1.3.0", advanced.ResolvedRef)
	s.Equal([]string{"v1.3.0"}, s.updateRuns(env, ns), "${REF} is the target ref, never the channel name")

	items, status := tc.List()
	s.Require().Equal(http.StatusOK, status)
	s.Require().Len(items, 1)
	s.Require().Len(items[0].Versions, 1, "an update moves the row; it never adds a successor")
	s.Equal("stable", items[0].Versions[0].Ref)
	s.Equal("v1.3.0", items[0].Versions[0].ResolvedRef)
}

// The installed release's manifest writes its update runs elsewhere, so only
// the target's own update steps can produce update-refs.
func (s *AdvanceSuite) TestAdvance_UpdateStepsRunFromTheTargetManifest() {
	target := s.stableManifest()
	installed := []byte(strings.ReplaceAll(string(target), updateRefs, "installed-update-refs"))
	f := s.newFixture("stable-tool", "v1.2.0", installed)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("stable")

	s.install(env, tc, ns)
	v130 := s.publish(f, "v1.3.0", target)

	s.update(tc, ns)
	s.waitAdvanced(tc, ns, v130)

	s.Equal([]string{"v1.3.0"}, s.updateRuns(env, ns))
	_, ranInstalled := s.workFile(env, ns, "installed-update-refs")
	s.False(ranInstalled, "the installed manifest's update steps must not run")
}

func (s *AdvanceSuite) TestAdvance_MovedMidUpdate_StaysOutdated() {
	f := s.rollingFixture()
	installed := s.tagCommit(f, "nightly")
	reads := &liveReads{returned: make(chan struct{}, 16)}
	env := s.NewEnv(kit.WithManifoldWrapper(func(inner manifold.Manifold) manifold.Manifold {
		reads.Manifold = inner
		return reads
	}))
	tc := env.TypedClient(s.T())
	ns := f.ns("nightly")

	s.install(env, tc, ns)
	first := s.moveTag(f, "nightly")

	s.touchWorkFile(env, ns, holdFile)
	s.update(tc, ns)
	env.WaitForState(s.T(), ns, domain.ArrowStateUpdating, wait)
	reads.drain()
	second := s.moveTag(f, "nightly")
	s.NotEqual(first, second)
	s.touchWorkFile(env, ns, releaseFile)

	reads.waitOne(s.T())
	reads.watchFetchesAt(first)
	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, wait)
	stale := s.detail(tc, ns)
	s.Equal(installed, stale.InstalledCommit, "the bits installed are not first's: nothing may be stamped")
	s.Require().NotNil(stale.Available, "the row stays outdated")
	s.Equal(first, stale.Available.Commit,
		"the bracket re-resolves without recording: the row still names the target the update began toward")
	s.True(stale.Outdated)
	s.Equal([]string{"nightly"}, s.updateRuns(env, ns), "the update steps did run")

	rechecked := s.checkAvailable(tc, ns)
	s.Require().NotNil(rechecked)
	s.Equal(second, rechecked.Commit, "the next check records where the tag moved to")

	s.update(tc, ns)
	s.waitAdvanced(tc, ns, second)
	s.Equal([]string{"nightly", "nightly"}, s.updateRuns(env, ns))
	s.Zero(reads.fetchesAt(), "a bracket that found its target moved fetches nothing to stamp it with")
}

// liveReads reports every live ref listing the daemon finished, which is how
// a test knows an update's bracket has re-resolved its target.
type liveReads struct {
	manifold.Manifold
	returned chan struct{}

	mu      sync.Mutex
	watched string
	fetches int
}

// watchFetchesAt starts counting manifest fetches at commit, the fetch an
// advance to it makes.
func (m *liveReads) watchFetchesAt(commit string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watched = commit
}

func (m *liveReads) fetchesAt() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fetches
}

func (m *liveReads) ResolveArrowAtCommit(
	ctx context.Context,
	ns domain.Namespace,
	ref string,
	commit string,
) (*domain.Arrow, []byte, string, error) {
	m.mu.Lock()
	if m.watched != "" && commit == m.watched {
		m.fetches++
	}
	m.mu.Unlock()
	return m.Manifold.ResolveArrowAtCommit(ctx, ns, ref, commit)
}

func (m *liveReads) FreshSnapshot(ctx context.Context, ns domain.Namespace) (domain.RefSnapshot, error) {
	snap, err := m.Manifold.FreshSnapshot(ctx, ns)
	select {
	case m.returned <- struct{}{}:
	default:
	}
	return snap, err
}

func (m *liveReads) drain() {
	for {
		select {
		case <-m.returned:
		default:
			return
		}
	}
}

func (m *liveReads) waitOne(t *testing.T) {
	t.Helper()
	select {
	case <-m.returned:
	case <-time.After(wait):
		t.Fatal("the update's bracket never re-resolved its target")
	}
}

// TestAdvance_EndToEndBracket_SecondUpdateIsANoOp walks the whole bracket
// through the API: the update itself re-resolves the moved tag, runs the
// steps, advances the row, clears the badge, and a second update finds
// nothing ahead and runs nothing.
func (s *AdvanceSuite) TestAdvance_EndToEndBracket_SecondUpdateIsANoOp() {
	f := s.rollingFixture()
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("nightly")

	s.install(env, tc, ns)
	moved := s.moveTag(f, "nightly")

	s.Require().Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodUpdate, nil),
		"a moved target starts an update")
	advanced := s.waitAdvanced(tc, ns, moved)
	s.Require().NotNil(advanced.LastReturn)
	s.Equal(domain.MethodUpdate, advanced.LastReturn.Method)
	s.Equal("success", advanced.LastReturn.Outcome)
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, wait)

	s.Equal(http.StatusOK, tc.Execute(ns, domain.MethodUpdate, nil),
		"nothing newer is an idempotent no-op: no runtime event will follow")

	again := s.detail(tc, ns)
	s.Equal(string(domain.ArrowStateReady), again.State, "nothing ahead: no update begins")
	s.Equal(advanced.LastReturn, again.LastReturn, "no new execution ran")
	s.Equal(moved, again.InstalledCommit)
	s.Equal([]string{"nightly"}, s.updateRuns(env, ns), "the second update ran no steps")
}

func (s *AdvanceSuite) TestAdvance_ConstraintNeverCrossesMajor() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.2.0", manifest)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("v1.*")

	s.install(env, tc, ns)
	s.publish(f, "v2.0.0", manifest)
	s.Nil(s.checkAvailable(tc, ns), "v2.0.0 is outside v1.*")

	v130 := s.publish(f, "v1.3.0", manifest)
	available := s.checkAvailable(tc, ns)
	s.Require().NotNil(available)
	s.Equal(dto.AvailableDTO{Ref: "v1.3.0", Commit: v130}, *available)

	s.update(tc, ns)
	advanced := s.waitAdvanced(tc, ns, v130)
	s.Equal("constraint", advanced.SelectorKind)
	s.Equal("v1.3.0", advanced.ResolvedRef)
	s.Equal([]string{"v1.3.0"}, s.updateRuns(env, ns))
	s.Nil(s.checkAvailable(tc, ns), "v1.3.0 is the highest v1.*, whatever v2 holds")
}

func (s *AdvanceSuite) TestAdvance_CommitPinNeverOutdated() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.2.0", manifest)
	pinned := s.tagCommit(f, "v1.2.0")
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns(pinned)

	s.install(env, tc, ns)
	s.moveTag(f, "v1.2.0")
	s.publish(f, "v1.3.0", manifest)

	s.Nil(s.checkAvailable(tc, ns), "a commit pin follows nothing that can move")
	s.Equal(http.StatusOK, tc.Execute(ns, domain.MethodUpdate, nil), "an update of a commit pin starts nothing")

	detail := s.detail(tc, ns)
	s.Equal("commit", detail.SelectorKind)
	s.Equal(pinned, detail.InstalledCommit)
	s.Nil(detail.Available)
	s.False(detail.Outdated)
	s.Equal(string(domain.ArrowStateReady), detail.State)
	s.Empty(s.updateRuns(env, ns), "an update of a commit pin runs nothing")
}

func (s *AdvanceSuite) TestAdvance_ForceMovedOrderedTagDetected() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.2.0", manifest)
	s.publish(f, "v1.3.0", manifest)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("v1.2.0")

	s.install(env, tc, ns)
	s.Nil(s.checkAvailable(tc, ns), "a pin is not outdated by a newer release")

	moved := s.moveTag(f, "v1.2.0")
	available := s.checkAvailable(tc, ns)
	s.Require().NotNil(available, "a pin detects its own tag being force-moved")
	s.Equal(dto.AvailableDTO{Ref: "v1.2.0", Commit: moved}, *available)

	s.update(tc, ns)
	advanced := s.waitAdvanced(tc, ns, moved)
	s.Equal("pin", advanced.SelectorKind)
	s.Equal("v1.2.0", advanced.ResolvedRef)
	s.Equal([]string{"v1.2.0"}, s.updateRuns(env, ns))
}

func (s *AdvanceSuite) TestAdvance_TwoSelectorsOneRef_AreIndependentRows() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.2.0", manifest)
	v130 := s.publish(f, "v1.3.0", manifest)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	channelNS := f.ns("stable")
	pinNS := f.ns("v1.3.0")

	s.install(env, tc, channelNS)
	s.install(env, tc, pinNS)
	s.NotEqual(s.workDir(env, channelNS), s.workDir(env, pinNS), "each row owns its workdir")
	_, channelInstalled := s.workFile(env, channelNS, installMarker)
	_, pinInstalled := s.workFile(env, pinNS, installMarker)
	s.True(channelInstalled)
	s.True(pinInstalled)

	v140 := s.publish(f, "v1.4.0", manifest)
	s.NotNil(s.checkAvailable(tc, channelNS))
	s.Nil(s.checkAvailable(tc, pinNS))

	s.update(tc, channelNS)
	advanced := s.waitAdvanced(tc, channelNS, v140)
	s.Equal("v1.4.0", advanced.ResolvedRef)

	pin := s.detail(tc, pinNS)
	s.Equal("v1.3.0", pin.ResolvedRef, "advancing one row leaves the other where it was")
	s.Equal(v130, pin.InstalledCommit)
	s.Equal(string(domain.ArrowStateReady), pin.State)
	s.Equal([]string{"v1.4.0"}, s.updateRuns(env, channelNS))
	s.Empty(s.updateRuns(env, pinNS))

	items, status := tc.List()
	s.Require().Equal(http.StatusOK, status)
	s.Require().Len(items, 1)
	resolved := map[string]string{}
	for _, v := range items[0].Versions {
		resolved[v.Ref] = v.ResolvedRef
	}
	s.Equal(map[string]string{"stable": "v1.4.0", "v1.3.0": "v1.3.0"}, resolved)
}

// A seeded row is the shape data written before selector kinds existed has:
// no stored kind and no resolved commit. It must act as a pin of its own ref.
func (s *AdvanceSuite) TestAdvance_LegacyZeroKindRow_ActsAsPin() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.2.0", manifest)
	tagged := s.tagCommit(f, "v1.2.0")
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("v1.2.0")

	s.Require().Equal(http.StatusCreated, tc.Seed(ns, manifest))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, wait)

	available := s.checkAvailable(tc, ns)
	s.Require().NotNil(available, "an unknown installed commit is not current")
	s.Equal(dto.AvailableDTO{Ref: "v1.2.0", Commit: tagged}, *available)
	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, wait)

	legacy := s.detail(tc, ns)
	s.Equal("pin", legacy.SelectorKind)
	s.Empty(legacy.InstalledCommit)

	s.update(tc, ns)
	s.waitAdvanced(tc, ns, tagged)
	s.Nil(s.checkAvailable(tc, ns), "once its commit is known, an unmoved pin is current")

	moved := s.moveTag(f, "v1.2.0")
	available = s.checkAvailable(tc, ns)
	s.Require().NotNil(available)
	s.Equal(dto.AvailableDTO{Ref: "v1.2.0", Commit: moved}, *available)

	s.Require().Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, wait)
	s.Require().Equal(http.StatusOK, tc.Remove(ns))

	items, status := tc.List()
	s.Require().Equal(http.StatusOK, status)
	s.Empty(items)
}

func (s *AdvanceSuite) TestAdvance_SecondUpdateWhileFirstRuns_IsRejected() {
	f := s.rollingFixture()
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("nightly")

	s.install(env, tc, ns)
	moved := s.moveTag(f, "nightly")

	s.touchWorkFile(env, ns, holdFile)
	s.update(tc, ns)
	env.WaitForState(s.T(), ns, domain.ArrowStateUpdating, wait)

	s.Equal(http.StatusUnprocessableEntity, tc.Execute(ns, domain.MethodUpdate, nil),
		"one row runs one update bracket at a time")

	s.touchWorkFile(env, ns, releaseFile)
	s.waitAdvanced(tc, ns, moved)
	s.Equal([]string{"nightly"}, s.updateRuns(env, ns))
}

func (s *AdvanceSuite) TestAdvance_RunningArrowIsStoppedBeforeAdvance() {
	f := s.rollingFixture()
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("nightly")

	s.install(env, tc, ns)
	moved := s.moveTag(f, "nightly")

	s.Require().Equal(http.StatusAccepted, tc.Execute(ns, "execute", nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateRunning, wait)
	// The stream's last PID may still be the install run's; wait for the
	// served process's own.
	running := kit.WaitForDetail(s.T(), tc, ns, "the served process recorded its PID", wait,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.ActiveRun != nil && d.ActiveRun.Method == domain.MethodExecute && d.ActiveRun.PID > 0
		})
	pid := running.ActiveRun.PID

	s.update(tc, ns)
	advanced := s.waitAdvanced(tc, ns, moved)

	events, ok := s.workFile(env, ns, "events")
	s.Require().True(ok)
	s.Equal([]string{"stop-signalled", "update-ran"}, strings.Fields(events),
		"the served process was told to stop before the update steps ran")
	s.False(env.ProcessAlive(pid))
	s.Contains(env.StateHistory(ns), string(domain.ArrowStateStopping), "the update went through a stop")
	s.Require().NotNil(advanced.LastReturn)
	s.Equal(domain.MethodUpdate, advanced.LastReturn.Method)
	s.Equal([]string{"nightly"}, s.updateRuns(env, ns))
}

// The target gains a service: the row lands outdated with the dependency sync
// pending, the sync installs the new dependency, and only then do the
// target's own update steps run. Both steps append to one log, so the order
// they ran in is recorded by the processes themselves.
func (s *AdvanceSuite) TestAdvance_GainedDependency_SyncedBeforeUpdateSteps() {
	logPath := filepath.Join(s.T().TempDir(), "events")
	testName := s.T().Name()
	slug := strings.ToLower(strings.ReplaceAll(testName[strings.LastIndex(testName, "/")+1:], "_", "-"))
	depKey := "quiver-test/gained-service-" + slug
	depNS := kit.NSFor(depKey, "v1")

	s.Repos.Set(depKey, kit.BuildUpgradeRepo(s.T(), gainedService(logPath)))
	s.T().Cleanup(func() { s.Repos.Delete(depKey) })
	f := s.newFixture("gains-dependency", "v1", gainsDependency(logPath, ""))
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("v*")

	s.install(env, tc, ns)

	var v2 string
	s.Repos.Mutate(func() {
		v2 = kit.AddV2ToRepo(s.T(), f.storer, gainsDependency(logPath, depNS))
	})

	s.update(tc, ns)
	advanced := s.waitAdvanced(tc, ns, v2)
	env.WaitForState(s.T(), depNS, domain.ArrowStateRunning, wait)

	s.Equal("v2", advanced.ResolvedRef)
	s.Require().NotNil(advanced.LastReturn)
	s.Equal(domain.MethodUpdate, advanced.LastReturn.Method)
	s.Equal("success", advanced.LastReturn.Outcome)

	events, err := os.ReadFile(logPath) // #nosec G304 -- a temp file this test owns
	s.Require().NoError(err)
	s.Equal([]string{"dependency-installed", "update-ran"}, strings.Fields(string(events)),
		"the gained dependency is installed before the update steps run")

	history := env.StateHistory(ns)
	outdatedAt := slices.Index(history, string(domain.ArrowStateOutdated))
	updatingAt := slices.Index(history, string(domain.ArrowStateUpdating))
	s.Require().NotEqual(-1, outdatedAt, "the row lands outdated with the sync pending: %v", history)
	s.Require().NotEqual(-1, updatingAt, "%v", history)
	s.Less(outdatedAt, updatingAt, "the update begins only from the outdated row: %v", history)
}

// gainedService is a long-running service whose install records itself in
// the shared log.
func gainedService(logPath string) []byte {
	return []byte(`schema: "arrow@v0"
metadata:
  name: quiver-test.gained-service
  description: A service a target gains
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: echo dependency-installed >> "` + logPath + `"
          title: Install
          timeout: 10s
          exit_on_failure: true
      execute:
        - type: run
          command: sleep 3600
          title: Serve
          timeout: 3700s
          exit_on_failure: false
      stop:
        - type: signal
          signal: graceful
          timeout: 10s
          exit_on_failure: false
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`)
}

// gainsDependency is the dependent's manifest; with a service it is the
// release that gains it, and its update step records itself in the log.
func gainsDependency(logPath, service string) []byte {
	services := ""
	if service != "" {
		services = "    services:\n      - " + service + "\n"
	}
	return []byte(`schema: "arrow@v0"
metadata:
  name: quiver-test.gains-dependency
  description: An arrow whose next release gains a service
targets:
  "*":
` + services + `    lifecycle:
      install:
        - type: run
          command: echo installed
          title: Install
          timeout: 10s
          exit_on_failure: true
      update:
        - type: run
          command: echo update-ran >> "` + logPath + `"
          title: Update
          timeout: 10s
          exit_on_failure: true
      execute:
        - type: run
          command: echo executed
          title: Execute
          timeout: 10s
          exit_on_failure: true
      stop:
        - type: signal
          signal: graceful
          timeout: 10s
          exit_on_failure: false
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`)
}
