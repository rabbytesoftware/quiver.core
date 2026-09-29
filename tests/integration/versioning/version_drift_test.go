//go:build integration

package versioning_test

import (
	"net/http"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

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
