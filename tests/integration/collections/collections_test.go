//go:build integration

package collections_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type CollectionSuite struct{ kit.IntegrationSuite }

func TestQuiverIntegration(t *testing.T) {
	suite.Run(t, new(CollectionSuite))
}

// --- follow / get ---

func (s *CollectionSuite) TestFollow_And_Get() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.CollectionNSFor("gaming-collection", "v1")

	s.Equal(http.StatusCreated, tc.CollectionFollow(ns))

	detail, status := tc.CollectionGet(ns)
	s.Equal(http.StatusOK, status)
	s.Equal("Gaming Quiver", detail.Name)
	s.Len(detail.Arrows, 2)
	s.True(detail.Followed)
}

func (s *CollectionSuite) TestGet_Uncached() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.CollectionNSFor("yaml-collection", "v1")

	detail, status := tc.CollectionGet(ns)
	s.Equal(http.StatusOK, status)
	s.Equal("YAML Quiver", detail.Name)
	s.False(detail.Followed)
}

// Follow caches all arrows; Get should show them resolved.
func (s *CollectionSuite) TestFollow_Resolves_Arrows() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.CollectionNSFor("gaming-collection", "v1")

	s.Equal(http.StatusCreated, tc.CollectionFollow(ns))

	detail, status := tc.CollectionGet(ns)
	s.Equal(http.StatusOK, status)
	s.Require().Len(detail.Arrows, 2)
	for _, a := range detail.Arrows {
		s.True(a.Resolved, "arrow %s should be resolved after follow", a.Namespace)
		s.NotEmpty(a.Name)
	}
}

// --- add arrow from quiver ---

// Follow a quiver that has a local path arrow, then Add that arrow.
// The arrow is already in the manifest cache from the Follow, so Add should succeed.
func (s *CollectionSuite) TestFollow_Then_Add_LocalPathArrow() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	quiverNS := kit.CollectionNSFor("gaming-collection", "v1")

	s.Equal(http.StatusCreated, tc.CollectionFollow(quiverNS))

	// cs2 is a local path arrow: path: servers/cs2@v1
	// derives to quiver.test/quiver-test/gaming-collection/cs2@v1
	cs2NS := "quiver.test/quiver-test/gaming-collection/cs2@v1"
	s.Equal(http.StatusCreated, tc.Add(cs2NS))

	env.WaitForListLen(s.T(), 1, 30*time.Second)

	detail, status := tc.GetDetail(cs2NS)
	s.Equal(http.StatusOK, status)
	s.Equal("quiver-test.cs2", detail.Name)
}

// Follow a quiver that has an external namespace arrow, then Add that arrow.
func (s *CollectionSuite) TestFollow_Then_Add_ExternalNamespaceArrow() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	quiverNS := kit.CollectionNSFor("gaming-collection", "v1")

	s.Equal(http.StatusCreated, tc.CollectionFollow(quiverNS))

	// tool-a is referenced by full namespace in the quiver manifest. The add
	// names a ref because quiver.test is not a known platform, so there is no
	// default branch list to fall back to when no stable release resolves.
	toolANS := "quiver.test/quiver-test/tool-a@v1"
	s.Equal(http.StatusCreated, tc.Add(toolANS))

	env.WaitForArrow(s.T(), toolANS, 30*time.Second)

	detail, status := tc.GetDetail(toolANS)
	s.Equal(http.StatusOK, status)
	s.NotEmpty(detail.Name)
}

// Follow a quiver → Add its arrow → Install it. Full blackbox path from quiver to running arrow.
func (s *CollectionSuite) TestFollow_Then_Add_Then_Install_CollectionArrow() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	quiverNS := kit.CollectionNSFor("gaming-collection", "v1")

	s.Equal(http.StatusCreated, tc.CollectionFollow(quiverNS))

	cs2NS := "quiver.test/quiver-test/gaming-collection/cs2@v1"
	s.Equal(http.StatusCreated, tc.Add(cs2NS))
	env.WaitForListLen(s.T(), 1, 30*time.Second)

	s.Equal(http.StatusAccepted, tc.Install(cs2NS, nil))
	env.WaitForState(s.T(), cs2NS, domain.ArrowStateReady, 60*time.Second)

	detail, status := tc.GetDetail(cs2NS)
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateReady), detail.State)
}

// --- seed → follow → add ---

// Seed a quiver manifest inline, follow it, then add one of its arrows.
// This proves the full seed → follow → install flow without any fixture quiver repos.
func (s *CollectionSuite) TestSeed_Then_Follow_Then_Add() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	quiverNS := "quiver.test/quiver-test/custom-quiver"

	manifest := []byte(`schema: "collection@v0"
metadata:
  name: "Custom Quiver"
  description: "Seeded inline for testing"
arrows:
  - namespace: quiver.test/quiver-test/tool-a
`)
	s.Equal(http.StatusCreated, tc.CollectionSeedManifest(quiverNS, manifest))

	s.Equal(http.StatusCreated, tc.CollectionFollow(quiverNS))

	detail, status := tc.CollectionGet(quiverNS)
	s.Equal(http.StatusOK, status)
	s.Equal("Custom Quiver", detail.Name)
	s.True(detail.Followed)

	// Add the arrow that was catalogued by Follow. The ref is named because
	// quiver.test is not a known platform, so a refless add has no default
	// branch list to fall back to.
	toolANS := "quiver.test/quiver-test/tool-a@v1"
	s.Equal(http.StatusCreated, tc.Add(toolANS))
	env.WaitForListLen(s.T(), 1, 30*time.Second)
}

// Seed a quiver, follow it, add two arrows from it in sequence.
func (s *CollectionSuite) TestSeed_Then_Follow_Then_Add_MultipleArrows() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	quiverNS := "quiver.test/quiver-test/multi-arrow-quiver"

	manifest := []byte(`schema: "collection@v0"
metadata:
  name: "Multi Arrow Quiver"
  description: "Has two arrows"
arrows:
  - namespace: quiver.test/quiver-test/tool-a
  - namespace: quiver.test/quiver-test/service-b
`)
	s.Equal(http.StatusCreated, tc.CollectionSeedManifest(quiverNS, manifest))
	s.Equal(http.StatusCreated, tc.CollectionFollow(quiverNS))

	s.Equal(http.StatusCreated, tc.Add("quiver.test/quiver-test/tool-a@v1"))
	s.Equal(http.StatusCreated, tc.Add("quiver.test/quiver-test/service-b@v1"))
	env.WaitForListLen(s.T(), 2, 30*time.Second)
}

// --- manifest endpoints ---

func (s *CollectionSuite) TestGetManifest_AfterFollow() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.CollectionNSFor("gaming-collection", "v1")

	s.Equal(http.StatusCreated, tc.CollectionFollow(ns))

	manifest, status := tc.CollectionGetManifest(ns)
	s.Equal(http.StatusOK, status)
	s.Equal("Gaming Quiver", manifest.Meta.Name)
	s.Len(manifest.Arrows, 2)
}

func (s *CollectionSuite) TestGetManifest_Seeded() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := "quiver.test/quiver-test/seeded-quiver"

	body := []byte(`schema: "collection@v0"
metadata:
  name: "Seeded Quiver"
  description: "A seeded quiver"
arrows:
  - namespace: quiver.test/quiver-test/tool-a
`)
	s.Equal(http.StatusCreated, tc.CollectionSeedManifest(ns, body))

	manifest, status := tc.CollectionGetManifest(ns)
	s.Equal(http.StatusOK, status)
	s.Equal("Seeded Quiver", manifest.Meta.Name)
	s.Require().Len(manifest.Arrows, 1)
	s.Equal("quiver.test/quiver-test/tool-a", manifest.Arrows[0].Namespace)
}

// --- list ---

func (s *CollectionSuite) TestList_FollowedFilter() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	gamingNS := kit.CollectionNSFor("gaming-collection", "v1")

	s.Equal(http.StatusCreated, tc.CollectionFollow(gamingNS))
	env.WaitForCollectionFollowed(s.T(), gamingNS, 5*time.Second)

	trueVal := true
	followed, status := tc.CollectionList(&trueVal)
	s.Equal(http.StatusOK, status)
	s.Len(followed, 1)
	s.Equal(gamingNS, followed[0].Namespace)
	s.True(followed[0].Followed)
	s.Equal(2, followed[0].ArrowCount)

	falseVal := false
	unfollowed, status := tc.CollectionList(&falseVal)
	s.Equal(http.StatusOK, status)
	for _, q := range unfollowed {
		s.NotEqual(gamingNS, q.Namespace)
		s.False(q.Followed)
	}

	all, status := tc.CollectionList(nil)
	s.Equal(http.StatusOK, status)
	found := false
	for _, q := range all {
		if q.Namespace == gamingNS {
			found = true
			break
		}
	}
	s.True(found, "gaming-collection should appear in all list")
}

// --- error paths ---

func (s *CollectionSuite) TestFollow_Duplicate_Returns409() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.CollectionNSFor("yaml-collection", "v1")

	s.Equal(http.StatusCreated, tc.CollectionFollow(ns))
	s.Equal(http.StatusConflict, tc.CollectionFollow(ns))
}

func (s *CollectionSuite) TestUnfollow_NotFollowed_Returns404() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.CollectionNSFor("gaming-collection", "v1")

	s.Equal(http.StatusNotFound, tc.CollectionUnfollow(ns))
}

func (s *CollectionSuite) TestUnfollow_Then_Refollow() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.CollectionNSFor("yaml-collection", "v1")

	s.Equal(http.StatusCreated, tc.CollectionFollow(ns))
	s.Equal(http.StatusOK, tc.CollectionUnfollow(ns))

	detail, status := tc.CollectionGet(ns)
	s.Equal(http.StatusOK, status)
	s.False(detail.Followed)

	s.Equal(http.StatusCreated, tc.CollectionFollow(ns))

	detail, status = tc.CollectionGet(ns)
	s.Equal(http.StatusOK, status)
	s.True(detail.Followed)
}

func (s *CollectionSuite) TestPartialArrowFailure() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.CollectionNSFor("missing-arrow-collection", "v1")

	s.Equal(http.StatusCreated, tc.CollectionFollow(ns))

	detail, status := tc.CollectionGet(ns)
	s.Equal(http.StatusOK, status)
	s.Require().Len(detail.Arrows, 1)
	s.False(detail.Arrows[0].Resolved)
}

// --- manifest validation ---

func (s *CollectionSuite) TestValidateManifest_Valid() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	body := []byte(`schema: "collection@v0"
metadata:
  name: "Valid Quiver"
  description: "passes validation"
arrows:
  - namespace: quiver.test/quiver-test/tool-a
`)
	result, status := tc.CollectionValidateManifest("quiver.test/quiver-test/any-quiver", body)
	s.Equal(http.StatusOK, status)
	s.True(result.Valid)
	s.Empty(result.Errors)
}

func (s *CollectionSuite) TestValidateManifest_InvalidSchema() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	result, status := tc.CollectionValidateManifest("quiver.test/quiver-test/any-quiver", []byte(`{}`))
	s.Equal(http.StatusUnprocessableEntity, status)
	s.False(result.Valid)
	s.NotEmpty(result.Errors)
}

func (s *CollectionSuite) TestValidateManifest_MissingArrows() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	body := []byte(`schema: "collection@v0"
metadata:
  name: "No Arrows"
  description: "missing arrows list"
`)
	result, status := tc.CollectionValidateManifest("quiver.test/quiver-test/any-quiver", body)
	s.Equal(http.StatusUnprocessableEntity, status)
	s.False(result.Valid)
	s.NotEmpty(result.Errors)
}
