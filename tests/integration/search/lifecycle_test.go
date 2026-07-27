//go:build integration

package search_test

import (
	"net/http"
	"time"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// TestLifecycle_SearchDiscoverStreamAddSearch walks the whole feature: an empty
// machine searches, discovers, streams, adds, and finds the arrow locally. The
// assertion that matters is not that the add succeeded but that it cost
// nothing — discovery already proved and cached the manifest.
func (s *SearchSuite) TestLifecycle_SearchDiscoverStreamAddSearch() {
	prov := newStubProvider(fixtureHost)
	gate := prov.gated("lumen", candidateFor("search-lumen", 128))

	var counter *countingManifold
	env := s.NewEnv(
		kit.WithProviders(prov),
		kit.WithManifoldWrapper(func(inner manifold.Manifold) manifold.Manifold {
			counter = &countingManifold{Manifold: inner}
			return counter
		}),
	)
	tc := env.TypedClient(s.T())

	// 1. A fresh install knows nothing.
	local, status := tc.Search("lumen", kit.SearchParams{})
	s.Require().Equal(http.StatusOK, status)
	s.Require().Empty(local)

	// 2. Discovery answers immediately, without waiting for the blocked host.
	job, status := tc.Discover("lumen")
	s.Require().Equal(http.StatusAccepted, status)
	s.Require().NotEmpty(job.JobID)

	// 3. Subscribe, release the host, and receive a renderable result.
	conn := s.dialJob(env, job.JobID)
	env.WaitDiscoveryRegistered()
	close(gate)

	frames := readResults(s.T(), conn, 1)
	streamed := frames[fixtureNS("search-lumen")]
	s.Equal("lumen", streamed.Name)
	s.Equal([]string{fixtureBranch}, streamed.Versions)
	s.Equal(models.ProvenanceSeen, streamed.Provenance)
	s.Equal(128, streamed.Stars)

	// 4. The socket closes and the summary is read once.
	s.Require().NoError(conn.Close())
	summary := s.waitForCompleted(tc, job.JobID)
	s.Equal(1, summary.Found)
	s.Equal(1, summary.Verified)
	s.Zero(summary.Skipped)

	resolvesAfterDiscovery, _ := counter.counts()
	s.Require().Equal(1, resolvesAfterDiscovery, "discovery proves each candidate exactly once")
	s.Require().Equal(1, prov.searches())

	// 5. Adding the discovered arrow serves from the warm vault cache.
	s.Require().Equal(http.StatusCreated, tc.Add(discoveredNS("search-lumen")))

	resolvesAfterAdd, parsesAfterAdd := counter.counts()
	s.Equal(resolvesAfterDiscovery, resolvesAfterAdd,
		"add must not resolve again — discovery already cached the manifest")
	s.Equal(1, prov.searches(), "add must not ask any provider anything")
	s.Positive(parsesAfterAdd, "the add path read the cached bytes rather than fetching them")

	// 6. The arrow is now a local, installed result.
	var installed []apidto.SearchResultDTO
	s.Require().Eventually(func() bool {
		got, st := tc.Search("lumen", kit.SearchParams{})
		if st != http.StatusOK || len(got) != 1 || !got[0].Installed {
			return false
		}
		installed = got
		return true
	}, catalogWait, 20*time.Millisecond)

	s.Require().Len(installed, 1)
	s.Equal(fixtureNS("search-lumen"), installed[0].Namespace)
	s.Equal(models.ProvenanceInstalled, installed[0].Provenance)
	s.Equal([]string{fixtureBranch}, installed[0].Versions)

	// Nothing above reached a host a second time.
	finalResolves, _ := counter.counts()
	s.Equal(1, finalResolves)
	s.Equal(1, prov.searches())
}

// TestLifecycle_Rediscovery_ReusesTheWarmIndex proves the claim that discovery
// cost falls with use: the second pass over the same repository re-verifies from
// the vault without any extra provider request beyond its own search.
func (s *SearchSuite) TestLifecycle_Rediscovery_KeepsOneResultPerNamespace() {
	prov := newStubProvider(fixtureHost).answer("widget", candidateFor("search-widget-beta", 10))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	s.runDiscovery(tc, "widget", 1)
	s.runDiscovery(tc, "widget", 1)

	s.Equal(2, prov.searches(), "one metered search per pass, no more")

	results, status := tc.Search("widget", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.Require().Len(results, 1, "re-saving a row is an idempotent upsert, not a duplicate")
	s.Equal([]string{fixtureBranch}, results[0].Versions)
}
