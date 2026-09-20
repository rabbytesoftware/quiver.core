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
