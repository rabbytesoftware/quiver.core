//go:build integration

package search_test

import (
	"net/http"
	"time"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// homeHost is the host the shipped shelves name. The fixture repositories live
// elsewhere, so the stub answers as this host while its candidates point at them.
const homeHost = "github.com"

// homeFilled waits until a refresh has filled every shelf and finished.
func (s *SearchSuite) homeFilled(tc *kit.TypedClient) apidto.HomeDTO {
	s.T().Helper()
	var filled apidto.HomeDTO
	s.Require().Eventually(func() bool {
		home, status := tc.Home()
		if status != http.StatusOK || home.Refreshing || len(home.Shelves) == 0 {
			return false
		}
		for _, shelf := range home.Shelves {
			if shelf.RefreshedAt == nil {
				return false
			}
		}
		filled = home
		return true
	}, catalogWait, 20*time.Millisecond)
	return filled
}

func (s *SearchSuite) TestHome_EmptyThenRefreshThenPopulated_ReadsNeverCallAHost() {
	prov := newStubProvider(homeHost).answer("",
		candidateFor("search-lumen", 7),
		manifestlessCandidate(),
		candidateFor("search-widget-alpha", 34),
	)
	var counter *countingManifold
	env := s.NewEnv(
		kit.WithProviders(prov),
		kit.WithManifoldWrapper(func(inner manifold.Manifold) manifold.Manifold {
			counter = &countingManifold{Manifold: inner}
			return counter
		}),
	)
	tc := env.TypedClient(s.T())

	empty, status := tc.Home()
	s.Require().Equal(http.StatusOK, status)
	s.Require().Len(empty.Shelves, 2)
	s.Equal("popular", empty.Shelves[0].ID)
	s.Equal("fresh", empty.Shelves[1].ID)
	for _, shelf := range empty.Shelves {
		s.Nil(shelf.RefreshedAt, "a shelf nobody filled has no timestamp")
		s.Empty(shelf.Arrows)
	}
	s.False(empty.Refreshing)

	refreshStatus, body := tc.RefreshHome()
	s.Equal(http.StatusAccepted, refreshStatus)
	s.Empty(body)

	home := s.homeFilled(tc)
	for _, shelf := range home.Shelves {
		s.Equal([]string{fixtureNS("search-lumen"), fixtureNS("search-widget-alpha")}, shelfNamespaces(shelf), shelf.ID)
		s.Equal(7, shelf.Arrows[0].Stars)
		s.True(shelf.Arrows[0].Known)
		s.False(shelf.Arrows[0].Installed)
		s.Equal([]string{fixtureBranch}, shelf.Arrows[0].Versions)
	}
	s.Equal("Popular", home.Shelves[0].Title)

	resolves := counter.fetches()
	s.Equal(3, resolves, "each candidate is resolved once across both shelves; the second shelf reads the vault")

	searches := prov.searches()
	s.Equal(2, searches, "one browse search per shelf")
	for range 5 {
		_, status := tc.Home()
		s.Require().Equal(http.StatusOK, status)
	}
	s.Equal(searches, prov.searches(), "reading home never reaches a host")
	again := counter.fetches()
	s.Equal(resolves, again, "reading home never resolves a manifest")

	s.Equal(http.StatusAccepted, statusOf(tc.RefreshHome()))
	s.Require().Eventually(func() bool { return prov.searches() == searches+2 }, catalogWait, 20*time.Millisecond)
	s.homeFilled(tc)
	repeated := counter.fetches()
	s.Equal(resolves, repeated, "a repeat refresh spends nothing on what the vault holds or knows to be absent")
}

func (s *SearchSuite) TestHome_RefreshBrowsesWithTheShelfSources() {
	prov := newStubProvider(homeHost).answer("", candidateFor("search-lumen", 7))
	env := s.NewEnv(kit.WithProviders(prov))
	tc := env.TypedClient(s.T())

	status, _ := tc.RefreshHome()
	s.Require().Equal(http.StatusAccepted, status)
	s.homeFilled(tc)

	prov.mu.Lock()
	defer prov.mu.Unlock()
	s.Require().Len(prov.calls, 2)
	limits := make([]int, 0, len(prov.calls))
	for _, call := range prov.calls {
		s.True(call.Unmarked)
		s.Empty(call.Text)
		limits = append(limits, call.Limit)
	}
	s.Equal([]int{100, 100}, limits, "every source asks its host for a full page")
}

// manifestlessCandidate is a repository whose fixture the resolver reports as
// definitively holding nothing installable, the outcome Fletcher's refusal has.
func manifestlessCandidate() provider.Candidate {
	return provider.Candidate{
		Namespace:     domain.Namespace(fixtureHost + "/acme/plain"),
		Name:          "plain",
		Source:        fixtureHost,
		DefaultBranch: fixtureBranch,
	}
}

func shelfNamespaces(shelf apidto.HomeShelfDTO) []string {
	out := make([]string, 0, len(shelf.Arrows))
	for _, arrow := range shelf.Arrows {
		out = append(out, arrow.Namespace)
	}
	return out
}

func statusOf(status int, _ []byte) int { return status }
