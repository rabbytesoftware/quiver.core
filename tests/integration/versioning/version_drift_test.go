//go:build integration

package versioning_test

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5/storage/memory"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// bareNS builds a refless test namespace: "quiver.test/<fixture>".
func bareNS(fixture string) string {
	return "quiver.test/" + fixture
}

// runnableYAML is BuildMinimalYAML plus an execute lifecycle, so a drifted
// arrow can be started.
func runnableYAML(name string) []byte {
	return []byte(`schema: "arrow@v0"
metadata:
  name: ` + name + `
  description: test
targets:
  "*":
    lifecycle:
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
`)
}

// releasedRepo registers a repo under key whose only release is v1.0.0.
func (s *VersioningSuite) releasedRepo(key string) *memory.Storage {
	storer := kit.BuildTaggedRepo(s.T(), "v1.0.0", runnableYAML("v1.0.0 content"))
	s.withUpgradeRepo(key, storer)
	return storer
}

func (s *VersioningSuite) release(storer *memory.Storage, tag string) {
	s.mutate(func() { kit.AddTaggedCommitToRepo(s.T(), storer, tag, runnableYAML(tag+" content")) })
}

func (s *VersioningSuite) addAndInstall(env *kit.Env, tc *kit.TypedClient, ns string) {
	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, wait)
}

// listState reports the state the catalog list gives the row filed under ref.
func (s *VersioningSuite) listState(tc *kit.TypedClient, key, ref string) string {
	items, status := tc.List()
	s.Require().Equal(http.StatusOK, status)
	for _, item := range items {
		if !strings.Contains(item.Namespace, key) {
			continue
		}
		for _, v := range item.Versions {
			if v.Ref == ref {
				return v.State
			}
		}
	}
	s.Failf("row missing from the list", "%s@%s", key, ref)
	return ""
}

// A refless add on a repository with no releases follows its default branch;
// a commit on that branch is an update.
func (s *VersioningSuite) TestVersionDrift_BranchOnlyRepo_NewCommitDetected() {
	key := "quiver-test/version-drift-branch"
	storer := kit.BuildBranchOnlyRepo(s.T(), kit.BuildMinimalYAML("Branch Tracked"))
	s.withUpgradeRepo(key, storer)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Require().Equal(http.StatusCreated, tc.Add(bareNS(key)))
	ns := kit.NSFor(key, "master")
	s.mutate(func() { kit.AddCommitToRepo(s.T(), storer, kit.BuildMinimalYAML("Branch Tracked v2")) })

	detail := kit.WaitForDetail(s.T(), tc, ns, "the branch's new commit recorded as available", wait,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.Outdated
		},
	)
	s.Require().NotNil(detail.Available)
	s.Equal("master", detail.Available.Ref)
	s.NotEqual(detail.InstalledCommit, detail.Available.Commit)
}

// The identity a refless add settles on is stored with its kind: a first
// release published later does not change what the row follows.
func (s *VersioningSuite) TestVersionDrift_BranchOnlyRepo_FirstReleaseKeepsTheIdentity() {
	key := "quiver-test/version-drift-branch-then-tag"
	storer := kit.BuildBranchOnlyRepo(s.T(), kit.BuildMinimalYAML("Branch Tracked"))
	s.withUpgradeRepo(key, storer)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Require().Equal(http.StatusCreated, tc.Add(bareNS(key)))
	ns := kit.NSFor(key, "master")
	var release string
	s.mutate(func() {
		kit.AddTaggedCommitToRepo(s.T(), storer, "v1.0.0", kit.BuildMinimalYAML("First Release"))
		release = kit.TagCommit(s.T(), storer, "v1.0.0")
	})

	// Nothing is installed from the row, so the check advances it at once.
	_, status := tc.CheckAvailable(ns)
	s.Require().Equal(http.StatusOK, status)
	detail := s.getDetail(tc, ns)
	s.Equal("channel", detail.SelectorKind)
	s.Equal("master", detail.ResolvedRef, "a branch row keeps following its branch")
	s.Equal(release, detail.InstalledCommit, "the branch moved with the release commit")
	s.Equal([]string{"master"}, s.versionsOf(tc, key))
}

// A pin follows exactly its ref: a newer release is not an update for it.
func (s *VersioningSuite) TestVersionDrift_Pin_NewerReleaseIsNotAnUpdate() {
	key := "quiver-test/version-drift-exact-pin"
	storer := s.releasedRepo(key)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(key, "v1.0.0")

	s.addAndInstall(env, tc, ns)
	s.release(storer, "v1.1.0")

	result, status := tc.CheckAvailable(ns)
	s.Require().Equal(http.StatusOK, status)
	s.Nil(result.Available)
	detail := s.getDetail(tc, ns)
	s.Equal("pin", detail.SelectorKind)
	s.False(detail.Outdated)
	s.Equal(string(domain.ArrowStateReady), detail.State)
}

// The badge the frontend renders reads the runtime state, a separate
// aggregate from the row's Available: a drift found by the passive check must
// move both, on the detail and on the list.
func (s *VersioningSuite) TestVersionDrift_InstalledArrow_RuntimeStateBecomesOutdated() {
	key := "quiver-test/version-drift-runtime-state"
	storer := s.releasedRepo(key)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(key, "stable")

	s.addAndInstall(env, tc, ns)
	s.release(storer, "v1.1.0")

	s.getDetail(tc, ns)
	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, wait)

	detail := kit.WaitForDetail(s.T(), tc, ns, "state=outdated alongside available", wait,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.State == string(domain.ArrowStateOutdated) && d.Outdated
		},
	)
	s.Require().NotNil(detail.Available)
	s.Equal("v1.1.0", detail.Available.Ref)
	s.Equal(string(domain.ArrowStateOutdated), s.listState(tc, key, "stable"),
		"the list the sidebar reads must report outdated too")
}

// Outdated is a badge, not a gate: a drifted idle arrow still starts.
func (s *VersioningSuite) TestVersionDrift_OutdatedArrow_StillStarts() {
	key := "quiver-test/version-drift-still-runnable"
	storer := s.releasedRepo(key)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(key, "stable")

	s.addAndInstall(env, tc, ns)
	s.release(storer, "v1.1.0")
	s.getDetail(tc, ns)
	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, wait)

	s.Require().Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodExecute, nil),
		"an outdated arrow is still installed and idle — starting it must not be refused")

	detail := kit.WaitForDetail(s.T(), tc, ns, "the _execute return recorded, back at state=outdated", wait,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK &&
				d.LastReturn != nil && d.LastReturn.Method == domain.MethodExecute &&
				d.State == string(domain.ArrowStateOutdated)
		},
	)
	s.Equal("success", detail.LastReturn.Outcome)
	s.True(detail.Outdated, "running an arrow does not resolve the drift that was found")
}

// A finished _execute lands a plain ready; the badge is reconciled back from
// the row's Available at once, not at the next hour-long check.
func (s *VersioningSuite) TestVersionDrift_RunningAnOutdatedArrow_KeepsTheBadge() {
	key := "quiver-test/version-drift-badge-survives-run"
	storer := s.releasedRepo(key)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(key, "stable")

	s.addAndInstall(env, tc, ns)
	s.release(storer, "v1.1.0")
	s.getDetail(tc, ns)
	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, wait)

	s.Require().Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodExecute, nil))
	kit.WaitForDetail(s.T(), tc, ns, "the _execute return recorded", wait,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.LastReturn != nil && d.LastReturn.Method == domain.MethodExecute
		},
	)
	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, wait)

	detail := s.getDetail(tc, ns)
	s.Equal(string(domain.ArrowStateOutdated), detail.State, "the run is over and the arrow is still behind")
	s.True(detail.Outdated)
	s.Equal(string(domain.ArrowStateOutdated), s.listState(tc, key, "stable"))
}

// liveSnapshots counts the live ref listings the daemon makes.
type liveSnapshots struct {
	manifold.Manifold
	calls atomic.Int32
}

func (m *liveSnapshots) FreshSnapshot(ctx context.Context, ns domain.Namespace) (domain.RefSnapshot, error) {
	m.calls.Add(1)
	return m.Manifold.FreshSnapshot(ctx, ns)
}

// The passive check runs at most once per TTL window: later reads inside the
// window do not reach the remote, so a release published meanwhile stays
// unseen until the next check is due.
func (s *VersioningSuite) TestVersionDrift_TTL_SecondCallWithinWindowDoesNotRecheckImmediately() {
	key := "quiver-test/version-drift-ttl"
	storer := s.releasedRepo(key)
	live := &liveSnapshots{}
	env := s.NewEnv(kit.WithManifoldWrapper(func(inner manifold.Manifold) manifold.Manifold {
		live.Manifold = inner
		return live
	}))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(key, "stable")

	s.addAndInstall(env, tc, ns)
	s.release(storer, "v1.1.0")

	kit.WaitForDetail(s.T(), tc, ns, "the first check's answer", wait,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.Available != nil && d.Available.Ref == "v1.1.0"
		},
	)
	s.Equal(int32(1), live.calls.Load())

	s.release(storer, "v1.2.0")
	for range 20 {
		detail := s.getDetail(tc, ns)
		s.Equal("v1.1.0", detail.Available.Ref, "a read inside the TTL window answers from the last check")
	}
	s.Equal(int32(1), live.calls.Load(), "no read inside the TTL window reached the remote")
}

// The explicit check reads the remote live: a release the snapshot cache has
// not seen yet is found at once.
func (s *VersioningSuite) TestVersionDrift_Check_ReadsTheRemoteLive() {
	key := "quiver-test/version-drift-live-check"
	storer := s.releasedRepo(key)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(key, "stable")

	s.addAndInstall(env, tc, ns)
	channels, status := tc.Channels(ns)
	s.Require().Equal(http.StatusOK, status)
	s.Require().NotEmpty(channels.Channels)
	s.Equal("v1.0.0", channels.Channels[0].Latest, "the cached snapshot primed before the release")

	s.release(storer, "v1.1.0")

	result, status := tc.CheckAvailable(ns)
	s.Require().Equal(http.StatusOK, status)
	s.Require().NotNil(result.Available)
	s.Equal("v1.1.0", result.Available.Ref)
}

// Channel discovery answers from the manifold's snapshot cache until its TTL
// passes, then reads the remote again. The clock is injected, so the TTL is
// crossed without a real wait.
func (s *VersioningSuite) TestVersionDrift_ManifoldCache_PastTTL_ChannelsReflectNewTag() {
	key := "quiver-test/version-drift-cache-ttl"
	storer := s.releasedRepo(key)
	clock := kit.NewAdvanceableClock(time.Now())
	env := s.NewEnv(kit.WithClock(clock.Now))
	tc := env.TypedClient(s.T())
	ns := kit.NSFor(key, "stable")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.release(storer, "v1.1.0")

	cached, status := tc.Channels(ns)
	s.Require().Equal(http.StatusOK, status)
	s.Require().NotEmpty(cached.Channels)
	s.Equal("v1.0.0", cached.Channels[0].Latest, "a fresh cache keeps answering from before the release")

	clock.Advance(2 * time.Hour)

	fresh, status := tc.Channels(ns)
	s.Require().Equal(http.StatusOK, status)
	s.Require().NotEmpty(fresh.Channels)
	s.Equal("v1.1.0", fresh.Channels[0].Latest, "past the TTL the remote is read again")
}
