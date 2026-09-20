//go:build integration

package versioning_test

import (
	"net/http"
	"strings"
	"time"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// bareNS builds a refless test namespace: "quiver.test/<fixture>", with no
// @ref at all. Refless is exactly what forces the daemon to pick a ref for
// itself — the resolution path this whole feature is about.
func bareNS(fixture string) string {
	return "quiver.test/" + fixture
}

// TestVersionDrift_BranchTracked_NoTags_DetectsCommitDrift installs a refless
// namespace against a repository that has never cut a release, forcing the
// default-branch fallback (RefIsBranch=true internally). The branch moves
// before the first-ever GetDetail call, so that single, TTL-unconstrained
// first check is enough to observe outdated flip from false to true, with no
// tag to recommend — plain branch drift.
func (s *VersioningSuite) TestVersionDrift_BranchTracked_NoTags_DetectsCommitDrift() {
	key := "quiver-test/version-drift-branch"
	storer := kit.BuildBranchOnlyRepo(s.T(), kit.BuildMinimalYAML("Branch Tracked"))
	s.withUpgradeRepo(key, storer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := bareNS(key)

	s.Require().Equal(http.StatusCreated, tc.Add(ns))

	// The branch moves after the arrow was resolved, before anyone has ever
	// asked GetDetail about it — the very first check must already see this.
	kit.AddCommitToRepo(s.T(), storer, kit.BuildMinimalYAML("Branch Tracked v2"))

	detail := kit.WaitForDetail(
		s.T(), tc, ns, "outdated=true with no recommended_ref (plain branch drift)",
		30*time.Second,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.Outdated
		},
	)
	s.True(detail.Outdated)
	s.Empty(detail.RecommendedRef, "a branch has no better named ref to switch to")
}

// TestVersionDrift_BranchTracked_TagAppears_RecommendsItInstead installs the
// same refless, tag-less namespace, but by the time the first check runs the
// repository has published its first real release. The check must recommend
// the tag rather than reporting branch drift — a branch is never again the
// answer once a repository has tags, however it got installed.
func (s *VersioningSuite) TestVersionDrift_BranchTracked_TagAppears_RecommendsItInstead() {
	key := "quiver-test/version-drift-branch-then-tag"
	storer := kit.BuildBranchOnlyRepo(s.T(), kit.BuildMinimalYAML("Branch Tracked"))
	s.withUpgradeRepo(key, storer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := bareNS(key)

	s.Require().Equal(http.StatusCreated, tc.Add(ns))

	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.0.0", kit.BuildMinimalYAML("First Release"))

	detail := kit.WaitForDetail(
		s.T(), tc, ns, "outdated=true recommending v1.0.0 over the branch",
		30*time.Second,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.Outdated
		},
	)
	s.Equal("v1.0.0", detail.RecommendedRef)
}

// TestVersionDrift_TagPinned_ExactPin_NewerTagRecommended installs an exact
// tag pin (no glob, so InstalledConstraint is empty) and confirms the passive
// check still recommends a newer stable tag once one exists — the behavior
// this feature generalizes beyond the existing constraint-only UpgradeRef
// flow.
func (s *VersioningSuite) TestVersionDrift_TagPinned_ExactPin_NewerTagRecommended() {
	key := "quiver-test/version-drift-exact-pin"
	storer := kit.BuildBranchOnlyRepo(s.T(), kit.BuildMinimalYAML("initial commit"))
	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.0.0", kit.BuildMinimalYAML("v1.0.0 content"))
	s.withUpgradeRepo(key, storer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := "quiver.test/" + key + "@v1.0.0"

	s.Require().Equal(http.StatusCreated, tc.Add(ns))

	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.1.0", kit.BuildMinimalYAML("v1.1.0 content"))

	detail := kit.WaitForDetail(
		s.T(), tc, ns, "outdated=true recommending v1.1.0",
		30*time.Second,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.Outdated
		},
	)
	s.Equal("v1.1.0", detail.RecommendedRef)
}

// TestVersionDrift_InstalledArrow_RuntimeStateBecomesOutdated is the end-to-end
// proof for the badge itself. Every other test in this file asserts
// `outdated`, which lives on the Arrow aggregate; the frontend badge reads
// `state`, which lives on the separate ArrowRuntime aggregate, and until this
// task nothing ever moved it — an arrow could sit at `outdated=true` with
// `state=ready` forever and the badge simply never appeared.
//
// So this installs the arrow for real (the earlier tests only add it, leaving
// no runtime aggregate to transition at all), then proves the transition three
// ways it actually reaches a client: over the live WebSocket runtime stream,
// on the detail endpoint, and on the list endpoint the sidebar reads.
func (s *VersioningSuite) TestVersionDrift_InstalledArrow_RuntimeStateBecomesOutdated() {
	key := "quiver-test/version-drift-runtime-state"
	storer := kit.BuildBranchOnlyRepo(s.T(), kit.BuildMinimalYAML("initial commit"))
	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.0.0", kit.BuildMinimalYAML("v1.0.0 content"))
	s.withUpgradeRepo(key, storer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := "quiver.test/" + key + "@v1.0.0"

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	// WaitForState listens on the WebSocket, so nothing has called GetDetail
	// yet and the one TTL-unconstrained check is still unclaimed when the
	// newer release lands.
	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.1.0", kit.BuildMinimalYAML("v1.1.0 content"))

	_, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status)

	// The badge's live path: the runtime transition has to reach clients over
	// the hub broadcast, not only on the next poll.
	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, 30*time.Second)

	// The runtime state is written before the catalog record on purpose, so
	// the two land a moment apart and the detail read waits for both. The
	// window reads as a badge that is a touch early — never as the missing
	// badge this path exists to fix.
	detail := kit.WaitForDetail(
		s.T(), tc, ns, "state=outdated alongside outdated=true",
		30*time.Second,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK &&
				d.State == string(domain.ArrowStateOutdated) && d.Outdated
		},
	)
	s.Equal(string(domain.ArrowStateOutdated), detail.State,
		"state is the field the badge reads, and it must have moved off ready")
	s.True(detail.Outdated, "the catalog record still lands too")
	s.Equal("v1.1.0", detail.RecommendedRef)

	// The sidebar lists arrows through GET /v0/arrow, whose per-version state
	// comes from the same runtime aggregate.
	items, listStatus := tc.List()
	s.Require().Equal(http.StatusOK, listStatus)
	var found bool
	for _, item := range items {
		if !strings.Contains(item.Namespace, key) {
			continue
		}
		for _, v := range item.Versions {
			if v.Ref != "v1.0.0" {
				continue
			}
			found = true
			s.Equal(string(domain.ArrowStateOutdated), v.State,
				"the list endpoint the sidebar reads must report outdated too")
		}
	}
	s.True(found, "the installed v1.0.0 must appear in the catalog list")
}

// runnableYAML is BuildMinimalYAML plus an execute lifecycle, and it exists
// because nothing in this suite had one. The two self-arrows this whole effort
// was built around declare no execute block at all — quiver.core's ARROW.md has
// only update, quiver.desktop's only preinstalled — so no test anywhere had
// ever started an arrow, from Outdated or from anywhere else, and the gap below
// shipped unnoticed.
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

// TestVersionDrift_OutdatedArrow_StillStarts is the regression for the whole
// point of an installed arrow: you can run it.
//
// Version-drift detection is passive — an ordinary GetDetail is enough to move
// an idle arrow to Outdated behind the user's back. Until this fix, Outdated
// was then treated as a gate rather than a badge: _execute declares no
// available_in, that branch demanded exactly Ready, and nothing ever cleared
// Outdated on its own. So every installed arrow in the system became
// permanently unstartable the moment upstream published a newer release and
// anyone opened its detail page — the product's central action, silently
// broken, until the user applied an update they never asked for. Applying an
// update is meant to be an explicit trigger, never a precondition for use.
//
// This drives the whole path a user would: install for real, let the passive
// check find genuine drift, then start it. Before the fix, Execute returned
// 409 and the state stayed at Outdated forever.
func (s *VersioningSuite) TestVersionDrift_OutdatedArrow_StillStarts() {
	key := "quiver-test/version-drift-still-runnable"
	storer := kit.BuildBranchOnlyRepo(s.T(), runnableYAML("initial commit"))
	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.0.0", runnableYAML("v1.0.0 content"))
	s.withUpgradeRepo(key, storer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := "quiver.test/" + key + "@v1.0.0"

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.1.0", runnableYAML("v1.1.0 content"))

	_, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status)
	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, 30*time.Second)

	// The arrow is now exactly where a user finds it after a release lands:
	// installed, idle, badged. Starting it must simply work.
	s.Require().Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodExecute, nil),
		"an outdated arrow is still installed and still idle — starting it must not be refused")

	// The run really happened: a completed _execute writes its return onto the
	// aggregate, which it can only reach by having gone through Running first.
	// And the badge is back by the time the run is readable as over — see
	// TestVersionDrift_RunningAnOutdatedArrow_KeepsTheBadge for the window
	// that used to sit here instead.
	detail := kit.WaitForDetail(
		s.T(), tc, ns, "the _execute return recorded, back at state=outdated",
		120*time.Second,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK &&
				d.LastReturn != nil && d.LastReturn.Method == domain.MethodExecute &&
				d.State == string(domain.ArrowStateOutdated)
		},
	)
	s.Equal("success", detail.LastReturn.Outcome, "the run itself has to have worked")

	// The drift fact survives the run on the catalog record that owns it, and
	// the runtime state it projects onto survives with it: the update the user
	// still has not applied is still there to apply, still badged.
	s.True(detail.Outdated, "running an arrow must not resolve the drift that was found")
	s.Equal("v1.1.0", detail.RecommendedRef)
}

// TestVersionDrift_RunningAnOutdatedArrow_KeepsTheBadge is the badge-window
// regression.
//
// EndExecution rewrites the runtime state from the method and outcome alone —
// a finished _execute lands on Ready, whatever the arrow's catalog record
// says. The catalog fact is untouched, so the detail endpoint, which reads it
// directly, went on reporting the drift; but the list and the WebSocket
// stream read ArrowRuntime.State, and those showed a plain, unbadged Ready
// arrow from the moment the run ended until the next TTL-gated version check
// came due — up to an hour of an arrow claiming to be current while the
// daemon knew otherwise.
//
// Running the arrow is the whole trigger, and running an arrow you have not
// updated yet is the ordinary case the previous fix in this branch deliberately
// made possible. So this drives it exactly as a user would and then reads the
// two views the badge actually reaches, with no wait between the run ending
// and the read beyond the run itself.
func (s *VersioningSuite) TestVersionDrift_RunningAnOutdatedArrow_KeepsTheBadge() {
	key := "quiver-test/version-drift-badge-survives-run"
	storer := kit.BuildBranchOnlyRepo(s.T(), runnableYAML("initial commit"))
	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.0.0", runnableYAML("v1.0.0 content"))
	s.withUpgradeRepo(key, storer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := "quiver.test/" + key + "@v1.0.0"

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.1.0", runnableYAML("v1.1.0 content"))

	_, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status)
	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, 30*time.Second)

	s.Require().Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodExecute, nil))

	// Wait for the run to be readable as over, then for the badge on the live
	// stream — the same hub broadcast a client sees, and in that order, which
	// is the order the dispatcher delivers them in.
	//
	// The TTL slot for this namespace was claimed by the GetDetail above and
	// is a full hour from being due again, so no version check can be what
	// restores this. Seconds on a broadcast rather than an hour on a TTL is
	// the entire difference the reconcile makes; the unit tests in
	// repositories/runtime/internal pin down that there is no window at all,
	// by reading the aggregate the instant the drain returns.
	kit.WaitForDetail(
		s.T(), tc, ns, "the _execute return recorded",
		120*time.Second,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK &&
				d.LastReturn != nil && d.LastReturn.Method == domain.MethodExecute
		},
	)
	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, 30*time.Second)

	detail := s.getDetail(tc, ns)
	s.Equal(string(domain.ArrowStateOutdated), detail.State,
		"the run is over and the arrow is still behind — the badge is back")
	s.True(detail.Outdated)
	s.Equal("v1.1.0", detail.RecommendedRef)

	// The list is the view the gap was actually visible in: the sidebar reads
	// per-version state off the same runtime aggregate.
	items, listStatus := tc.List()
	s.Require().Equal(http.StatusOK, listStatus)
	var found bool
	for _, item := range items {
		if !strings.Contains(item.Namespace, key) {
			continue
		}
		for _, v := range item.Versions {
			if v.Ref != "v1.0.0" {
				continue
			}
			found = true
			s.Equal(string(domain.ArrowStateOutdated), v.State,
				"the list the sidebar reads must badge a run arrow that is still behind")
		}
	}
	s.True(found, "the installed v1.0.0 must appear in the catalog list")
}

// TestVersionDrift_TTL_SecondCallWithinWindowDoesNotRecheckImmediately proves
// the TTL claim is real, not just present in code: once the first check has
// landed, a fixture change and an immediate follow-up call must not flip the
// signal again right away — only the exact TTL boundary (proven precisely by
// the unit tests with a fake clock) decides when the next check is due, and
// the default is a full hour, far longer than this test can or should wait.
func (s *VersioningSuite) TestVersionDrift_TTL_SecondCallWithinWindowDoesNotRecheckImmediately() {
	key := "quiver-test/version-drift-ttl"
	storer := kit.BuildBranchOnlyRepo(s.T(), kit.BuildMinimalYAML("Branch Tracked"))
	s.withUpgradeRepo(key, storer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := bareNS(key)

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	kit.AddCommitToRepo(s.T(), storer, kit.BuildMinimalYAML("Branch Tracked v2"))

	detail := kit.WaitForDetail(
		s.T(), tc, ns, "outdated=true after the first, TTL-unconstrained check",
		30*time.Second,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.Outdated
		},
	)
	s.Empty(detail.RecommendedRef)

	// A real release appears right after the TTL slot was already claimed.
	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.0.0", kit.BuildMinimalYAML("First Release"))

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(2 * time.Second)
	for {
		got, status := tc.GetDetail(ns)
		s.Require().Equal(http.StatusOK, status)
		s.True(got.Outdated, "the earlier branch-drift answer must not be cleared")
		s.Empty(got.RecommendedRef, "the new tag must not appear until the TTL is due again")

		select {
		case <-ticker.C:
		case <-deadline:
			return
		}
	}
}
