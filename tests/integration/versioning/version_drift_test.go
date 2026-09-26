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

// bareNS builds a refless test namespace: "quiver.test/<fixture>".
func bareNS(fixture string) string {
	return "quiver.test/" + fixture
}

// TestVersionDrift_BranchTracked_NoTags_DetectsCommitDrift proves plain
// branch drift: a refless namespace against a tag-less repo whose branch
// moves before the first GetDetail must flip outdated with no recommended ref.
func (s *VersioningSuite) TestVersionDrift_BranchTracked_NoTags_DetectsCommitDrift() {
	key := "quiver-test/version-drift-branch"
	storer := kit.BuildBranchOnlyRepo(s.T(), kit.BuildMinimalYAML("Branch Tracked"))
	s.withUpgradeRepo(key, storer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := bareNS(key)

	s.Require().Equal(http.StatusCreated, tc.Add(ns))

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

// TestVersionDrift_BranchTracked_TagAppears_RecommendsItInstead proves a
// branch-tracked namespace recommends a tag once one exists, never plain
// branch drift, once a repository has published its first release.
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

// TestVersionDrift_TagPinned_ExactPin_NewerTagRecommended proves the passive
// check recommends a newer tag even for an exact pin with no constraint set.
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

// TestVersionDrift_InstalledArrow_RuntimeStateBecomesOutdated proves the
// badge itself moves: the frontend reads ArrowRuntime.State, a separate
// aggregate from the Arrow.Outdated flag every other test in this file
// asserts, over the WebSocket stream, the detail endpoint, and the list.
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

	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.1.0", kit.BuildMinimalYAML("v1.1.0 content"))

	_, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status)

	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, 30*time.Second)

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

// runnableYAML is BuildMinimalYAML plus an execute lifecycle, needed because
// no fixture in this suite had one to test starting an arrow at all.
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

// TestVersionDrift_OutdatedArrow_StillStarts proves Outdated is a badge, not
// a gate: an idle arrow drifted to Outdated by a passive GetDetail must still
// start, since a version check must never block the product's central action.
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

	s.Require().Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodExecute, nil),
		"an outdated arrow is still installed and still idle — starting it must not be refused")

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

	s.True(detail.Outdated, "running an arrow must not resolve the drift that was found")
	s.Equal("v1.1.0", detail.RecommendedRef)
}

// TestVersionDrift_RunningAnOutdatedArrow_KeepsTheBadge proves a finished
// _execute does not clear the Outdated badge: EndExecution lands a plain
// Ready from the method and outcome alone, so the badge must be reconciled
// back from the catalog record, not left to the next hour-long TTL check.
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
// a follow-up call within the TTL window must not recheck: a fixture change
// right after the first check must stay invisible until the next TTL is due.
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

// TestVersionDrift_ManifoldCache_PastTTL_ResolvesConstraintLiveAndReflectsNewTag
// is the end-to-end regression guard for the manifold resolution-cache TTL
// mechanism: manifold.ResolveConstraint caches its answer for m.cacheTTL, an
// instance field threaded in through New/NewWithClock rather than a package
// constant, which is what makes it possible for production wiring
// (internal/engine/container.go) to tie it to the arrow store's own
// drift-check throttle (config.GetArrows().VersionCheckTTL) instead of an
// independently-chosen value that could silently drift apart from it. This
// test exercises that same instance-level TTL mechanism through the fixture
// manifold this suite already builds (tests/kit's NewWithResolversAndClock),
// whose cacheTTL is defaultManifoldCacheTTL (1h) -- not a live read of
// config.GetArrows().VersionCheckTTL, which container.go's own wiring is not
// reached by this suite at all. The two happen to share the same 1h value
// today; container.go's config-to-constructor wiring itself is covered by
// manual parity with store.go's identical resolveVersionCheckTTL shape, not
// by an automated test reaching through the real container. This exercises
// ResolveConstraint directly through PATCH .../:ns {"UpgradeRef": true} (no
// drift-check throttle involved at all, unlike
// TestVersionDrift_TTL_SecondCallWithinWindowDoesNotRecheckImmediately
// above), using an injected, manually-advanceable clock (kit.WithClock) so
// it proves the TTL boundary without a real wait.
func (s *VersioningSuite) TestVersionDrift_ManifoldCache_PastTTL_ResolvesConstraintLiveAndReflectsNewTag() {
	v1Content := kit.ReadFixture(s.T(), "versioned/v1/arrow.yaml")
	v2Content := kit.ReadFixture(s.T(), "versioned/v2/arrow.yaml")

	key := "quiver-test/version-drift-cache-ttl"
	upgradeStorer := kit.BuildUpgradeRepo(s.T(), v1Content)
	s.withUpgradeRepo(key, upgradeStorer)

	clock := kit.NewAdvanceableClock(time.Now())
	env := s.NewEnv(kit.WithClock(clock.Now))
	tc := env.TypedClient(s.T())

	ns := kit.NSForGlob(key, "v*")
	s.Require().Equal(http.StatusCreated, tc.Add(ns))

	v1ns := kit.NSFor(key, "v1")
	s.Require().Equal(http.StatusAccepted, tc.Install(v1ns, nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), v1ns, domain.ArrowStateReady, 120*time.Second)

	// This first UpgradeRef call is the one that actually primes
	// ResolveConstraint's cache entry for (v1ns, "v*"): Add's own
	// ResolveConstraint call above was keyed by the glob namespace, a
	// different cache key, so it does not share this entry. Since v2
	// does not exist in the repo yet, this resolves live to "v1" and,
	// finding the ref unchanged, only refreshes the manifest.
	s.Require().Equal(http.StatusOK, tc.Update(v1ns, map[string]any{"UpgradeRef": true}))

	// v2 now exists in the repo, but the manifold's cache -- still fresh --
	// must keep answering "v1" for this exact (namespace, pattern) pair.
	kit.AddV2ToRepo(s.T(), upgradeStorer, v2Content)
	s.Require().Equal(http.StatusOK, tc.Update(v1ns, map[string]any{"UpgradeRef": true}))
	stillV1 := s.getDetail(tc, v1ns)
	s.True(strings.HasSuffix(stillV1.Namespace, "@v1"),
		"must still be pinned at v1 while the manifold's ResolveConstraint cache is fresh, got: %s", stillV1.Namespace)

	// Advance the clock past the manifold cache's TTL -- this fixture
	// manifold's defaultManifoldCacheTTL (1h), not a live read of
	// config.GetArrows().VersionCheckTTL (see the doc comment above), no
	// real sleep needed -- and confirm the next UpgradeRef resolves the
	// constraint live and picks up v2.
	clock.Advance(2 * time.Hour)

	s.Require().Equal(http.StatusOK, tc.Update(v1ns, map[string]any{"UpgradeRef": true}))
	v2ns := kit.NSFor(key, "v2")
	env.WaitForState(s.T(), v2ns, domain.ArrowStateOutdated, 120*time.Second)
}
