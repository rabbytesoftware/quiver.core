//go:build integration

package search_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

// SearchSuite covers both search lanes: the offline catalog and vault query,
// and the provider-backed discovery pass with its stream and job summary.
type SearchSuite struct{ kit.IntegrationSuite }

func TestSearchIntegration(t *testing.T) {
	suite.Run(t, new(SearchSuite))
}

// catalogWait bounds waiting for the catalog projection to land.
const catalogWait = 60 * time.Second

// namespacesOf lists the result namespaces in rank order.
func namespacesOf(results []apidto.SearchResultDTO) []string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.Namespace)
	}
	return out
}

// resultFor finds the result for a bare namespace.
func resultFor(
	results []apidto.SearchResultDTO,
	ns string,
) (apidto.SearchResultDTO, bool) {
	for _, r := range results {
		if r.Namespace == ns {
			return r, true
		}
	}
	return apidto.SearchResultDTO{}, false
}

// ---------------------------------------------------------------- Lane A ----

func (s *SearchSuite) TestSearch_Query_EmptyCatalogReturnsNoResults() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	results, status := tc.Search("lumen", kit.SearchParams{})

	s.Equal(http.StatusOK, status)
	s.Empty(results, "a fresh install has nothing to answer with")
}

func (s *SearchSuite) TestSearch_Query_CatalogHitIsInstalled() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/search-lumen", "v1")

	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	env.WaitForArrow(s.T(), ns, catalogWait)

	results, status := tc.Search("lumen", kit.SearchParams{})

	s.Equal(http.StatusOK, status)
	s.Require().Len(results, 1)
	s.Equal(fixtureNS("search-lumen"), results[0].Namespace)
	s.Equal("lumen", results[0].Name)
	s.Equal("Lumen renders luminous scenes", results[0].Description)
	s.Equal([]string{"graphics", "render"}, results[0].Tags)
	s.Equal([]string{"v1"}, results[0].Versions)
	s.True(results[0].Installed)
	s.True(results[0].Known)
	s.Equal(models.ProvenanceInstalled, results[0].Provenance)
}

func (s *SearchSuite) TestSearch_Query_VaultHitIsSeenNotInstalled() {
	prov := newStubProvider(fixtureHost).answer("lumen", candidateFor("search-lumen", 42))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	s.runDiscovery(tc, "lumen", 1)

	results, status := tc.Search("lumen", kit.SearchParams{})

	s.Equal(http.StatusOK, status)
	s.Require().Len(results, 1)
	s.Equal(fixtureNS("search-lumen"), results[0].Namespace)
	s.Equal([]string{fixtureBranch}, results[0].Versions)
	s.False(results[0].Installed, "a discovered arrow is not installed")
	s.True(results[0].Known)
	s.Equal(models.ProvenanceSeen, results[0].Provenance)
	s.Equal(42, results[0].Stars)
	s.Equal(fixtureHost, results[0].Source)
}

func (s *SearchSuite) TestSearch_Query_CatalogWinsTheCollision() {
	prov := newStubProvider(fixtureHost).answer("lumen", candidateFor("search-lumen", 99))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	// The same namespace lands in both stores: discovery files it at the
	// branch, the add files it at the tag.
	s.runDiscovery(tc, "lumen", 1)
	ns := kit.NSFor("quiver-test/search-lumen", "v1")
	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	env.WaitForArrow(s.T(), ns, catalogWait)

	results, status := tc.Search("lumen", kit.SearchParams{})

	s.Equal(http.StatusOK, status)
	s.Require().Len(results, 1, "a namespace in both stores collapses to one result")
	s.Equal(fixtureNS("search-lumen"), results[0].Namespace)
	s.True(results[0].Installed, "the catalog row wins the collision")
	s.Equal(models.ProvenanceInstalled, results[0].Provenance)
	s.Zero(results[0].Stars, "the vault row contributes nothing, not even its stars")
}

func (s *SearchSuite) TestSearch_Query_RefsCollapseOntoOneResult() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	for _, ref := range []string{"v1", "v2"} {
		ns := kit.NSFor("quiver-test/search-multi", ref)
		s.Require().Equal(http.StatusCreated, tc.Add(ns))
	}
	env.WaitForArrow(s.T(), kit.NSFor("quiver-test/search-multi", "v1"), catalogWait)

	s.Require().Eventually(func() bool {
		results, _ := tc.Search("multiverse", kit.SearchParams{})
		return len(results) == 1 && len(results[0].Versions) == 2
	}, catalogWait, 20*time.Millisecond)

	results, status := tc.Search("multiverse", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.Require().Len(results, 1, "every ref of a namespace collapses onto one result")
	s.Equal(fixtureNS("search-multi"), results[0].Namespace)
	s.Equal([]string{"v1", "v2"}, results[0].Versions)
}

func (s *SearchSuite) TestSearch_OS_FiltersOnTheCompatibleList() {
	prov := newStubProvider(fixtureHost).answer("winonly", candidateFor("search-winonly", 1))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	s.runDiscovery(tc, "winonly", 1)

	unfiltered, status := tc.Search("winonly", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.Require().Len(unfiltered, 1)
	s.Equal(
		[]string{string(domain.OSWindowsAMD64), string(domain.OSWindowsARM64)},
		unfiltered[0].CompatibleOS,
		"an arrow with no target for this machine is still a verified arrow",
	)

	matching, status := tc.Search("winonly", kit.SearchParams{OS: string(domain.OSWindowsAMD64)})
	s.Equal(http.StatusOK, status)
	s.Len(matching, 1, "the os filter keeps an arrow that declares that platform")

	other, status := tc.Search("winonly", kit.SearchParams{OS: string(domain.OSLinuxAMD64)})
	s.Equal(http.StatusOK, status)
	s.Empty(other, "the os filter drops an arrow that declares no target for it")
}

func (s *SearchSuite) TestSearch_Limit_HonouredAndDefaulted() {
	prov := newStubProvider(fixtureHost).answer(
		"widget",
		candidateFor("search-widget-beta", 1),
		candidateFor("search-widget-alpha", 1),
	)
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	s.runDiscovery(tc, "widget", 2)

	testCases := []struct {
		name   string
		params kit.SearchParams
		want   int
	}{
		{name: "absent limit falls back to the default", params: kit.SearchParams{}, want: 2},
		{name: "limit one truncates", params: kit.SearchParams{Limit: "1"}, want: 1},
		{name: "limit above the set returns all", params: kit.SearchParams{Limit: "50"}, want: 2},
		{name: "unreadable limit falls back to the default", params: kit.SearchParams{Limit: "banana"}, want: 2},
		{name: "zero limit falls back to the default", params: kit.SearchParams{Limit: "0"}, want: 2},
		{name: "negative limit falls back to the default", params: kit.SearchParams{Limit: "-3"}, want: 2},
	}

	for _, tc2 := range testCases {
		s.Run(tc2.name, func() {
			results, status := tc.Search("widget", tc2.params)
			s.Equal(http.StatusOK, status)
			s.Len(results, tc2.want)
		})
	}
}

// --------------------------------------------------------------- Ranking ----

func (s *SearchSuite) TestSearch_Ranking_CurationOutranksAStranger() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	// The stranger is installed, so it answers from the catalog; the curated
	// arrow is only cached, so it answers from the vault — which is exactly the
	// case the followed-collection boost exists for. Each source normalises
	// against itself, so both arrive with the same relevance and only the boost
	// separates them. The stranger sorts first by namespace, so the tiebreak
	// favours it and the boost is the only thing that can flip the order.
	strangerNS := kit.NSFor("quiver-test/search-widget-alpha", "v1")
	s.Require().Equal(http.StatusCreated, tc.Add(strangerNS))
	env.WaitForArrow(s.T(), strangerNS, catalogWait)

	collectionNS := kit.CollectionNSFor("search-collection", "v1")
	s.Require().Equal(http.StatusCreated, tc.CollectionFollow(collectionNS))
	env.WaitForCollectionFollowed(s.T(), collectionNS, catalogWait)

	results, status := tc.Search("widget", kit.SearchParams{})

	s.Equal(http.StatusOK, status)
	s.Require().Len(results, 2)
	s.Equal(
		[]string{fixtureNS("search-widget-beta"), fixtureNS("search-widget-alpha")},
		namespacesOf(results),
		"a followed-collection arrow outranks an equally matching stranger",
	)

	curated, ok := resultFor(results, fixtureNS("search-widget-beta"))
	s.Require().True(ok)
	s.Equal(models.ProvenanceCollection, curated.Provenance,
		"collection membership is the provenance, and it is not readable off the catalog")
	s.False(curated.Installed)

	stranger, ok := resultFor(results, fixtureNS("search-widget-alpha"))
	s.Require().True(ok)
	s.Equal(models.ProvenanceInstalled, stranger.Provenance)
	s.True(stranger.Installed)
}

func (s *SearchSuite) TestSearch_Ranking_ExactNamePinBeatsAPartialMatch() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	for _, fixture := range []string{"search-lumen", "search-lumendb"} {
		ns := kit.NSFor("quiver-test/"+fixture, "v1")
		s.Require().Equal(http.StatusCreated, tc.Add(ns))
		env.WaitForArrow(s.T(), ns, catalogWait)
	}

	s.Require().Eventually(func() bool {
		results, _ := tc.Search("lumen", kit.SearchParams{})
		return len(results) == 2
	}, catalogWait, 20*time.Millisecond)

	results, status := tc.Search("lumen", kit.SearchParams{})
	s.Equal(http.StatusOK, status)
	s.Require().Len(results, 2)
	s.Equal(fixtureNS("search-lumen"), results[0].Namespace,
		"an exact name match states intent and outranks a partial one")
}

// ------------------------------------------------------------- Bad paths ----

func (s *SearchSuite) TestSearch_Query_BlankIsRejected() {
	env := s.NewEnv()
	c := env.Client(s.T())

	testCases := []struct {
		name  string
		query string
	}{
		{name: "empty", query: ""},
		{name: "spaces", query: "   "},
		{name: "tab", query: "\t"},
		{name: "newline", query: "\n"},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			resp := c.Search(tc.query, kit.SearchParams{})
			defer resp.Body.Close()
			s.Equal(http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func (s *SearchSuite) TestSearch_OS_UnknownValueIsRejected() {
	env := s.NewEnv()
	c := env.Client(s.T())

	resp := c.Search("lumen", kit.SearchParams{OS: "solaris/sparc"})
	defer resp.Body.Close()

	s.Equal(http.StatusBadRequest, resp.StatusCode)
}

// runDiscovery starts a pass, waits for it to complete, and asserts how many
// candidates it verified. It exists so Lane A tests can populate the vault
// through the product's own path without duplicating the wait. Every verified
// row is written to the vault before it is streamed, so a completed pass means
// the arrow is already answerable offline.
func (s *SearchSuite) runDiscovery(
	tc *kit.TypedClient,
	query string,
	wantVerified int,
) apidto.DiscoveryJobDTO {
	s.T().Helper()

	job, status := tc.Discover(query)
	s.Require().Equal(http.StatusAccepted, status)
	s.Require().NotEmpty(job.JobID)

	summary := s.waitForCompleted(tc, job.JobID)
	s.Require().Equal(wantVerified, summary.Verified)
	return summary
}

// waitForCompleted reads the job until the pass has finished. A client reads it
// once for real; the loop is here only because a test has no other way to learn
// the pass ended.
func (s *SearchSuite) waitForCompleted(
	tc *kit.TypedClient,
	jobID string,
) apidto.DiscoveryJobDTO {
	s.T().Helper()

	var summary apidto.DiscoveryJobDTO
	s.Require().Eventually(func() bool {
		got, status := tc.DiscoveryJob(jobID)
		if status != http.StatusOK || got.Status != string(usecases.JobCompleted) {
			return false
		}
		summary = got
		return true
	}, streamFrameTimeout, 10*time.Millisecond)
	return summary
}

// ------------------------------------------------------------------- Seed ----

// TestSearch_Seed_IndexesTheVaultRow proves a seeded arrow is answerable from
// both stores. While it holds a catalog row the catalog answers, which hides
// whether the vault was indexed at all; removing it from the catalog is what
// exposes the vault row, and a Seed that cached the bytes without their index
// metadata leaves nothing behind to find.
func (s *SearchSuite) TestSearch_Seed_IndexesTheVaultRow() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/search-lumen", "v1")
	content := kit.ReadFixture(s.T(), "search-lumen/arrow.yaml")

	s.Require().Equal(http.StatusCreated, tc.Seed(ns, content))
	env.WaitForArrow(s.T(), ns, catalogWait)

	seeded, status := tc.Search("lumen", kit.SearchParams{})
	s.Require().Equal(http.StatusOK, status)
	s.Require().Len(seeded, 1)
	s.Equal(fixtureNS("search-lumen"), seeded[0].Namespace)
	s.True(seeded[0].Installed, "a seeded arrow holds a catalog row, so the catalog answers")

	s.Require().Equal(http.StatusOK, tc.Remove(ns))

	var cached []apidto.SearchResultDTO
	s.Require().Eventually(func() bool {
		got, st := tc.Search("lumen", kit.SearchParams{})
		if st != http.StatusOK || len(got) != 1 || got[0].Installed {
			return false
		}
		cached = got
		return true
	}, catalogWait, 20*time.Millisecond,
		"the seeded manifest stays in the vault, so search must still answer with it")

	s.Equal(fixtureNS("search-lumen"), cached[0].Namespace)
	s.Equal("lumen", cached[0].Name)
	s.Equal("Lumen renders luminous scenes", cached[0].Description)
	s.Equal([]string{"graphics", "render"}, cached[0].Tags)
	s.Equal([]string{"v1"}, cached[0].Versions, "the vault row is filed at the ref it was seeded under")
	s.Equal(models.ProvenanceSeen, cached[0].Provenance)
	s.True(cached[0].Known)
	s.NotEmpty(cached[0].CompatibleOS, "the index carries the platforms the manifest declares")
}

// The OS filter reads the vault's per-ref platform rows, which only exist when
// the seed wrote index metadata alongside the bytes.
func (s *SearchSuite) TestSearch_Seed_VaultRowCarriesThePlatformList() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/search-winonly", "v1")
	content := kit.ReadFixture(s.T(), "search-winonly/arrow.yaml")

	s.Require().Equal(http.StatusCreated, tc.Seed(ns, content))
	env.WaitForArrow(s.T(), ns, catalogWait)
	s.Require().Equal(http.StatusOK, tc.Remove(ns))

	s.Require().Eventually(func() bool {
		got, st := tc.Search("winonly", kit.SearchParams{OS: string(domain.OSWindowsAMD64)})
		return st == http.StatusOK && len(got) == 1
	}, catalogWait, 20*time.Millisecond,
		"the os filter keeps a seeded arrow that declares that platform")

	other, status := tc.Search("winonly", kit.SearchParams{OS: string(domain.OSLinuxAMD64)})
	s.Equal(http.StatusOK, status)
	s.Empty(other, "the os filter drops a seeded arrow that declares no target for it")
}
