//go:build integration

package collections_test

import (
	"encoding/json"
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

	// cs2 is a local path arrow: path: servers/cs2. It lives inside the
	// collection's repository, so it inherits the collection's own ref.
	cs2NS := "quiver.test/quiver-test/gaming-collection/cs2@v1"
	s.Equal(http.StatusCreated, tc.Add(cs2NS))

	env.WaitForListLen(s.T(), 1, 120*time.Second)

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
	// names the ref the fixture repo tags, so the arrow is pinned to it.
	toolANS := "quiver.test/quiver-test/tool-a@v1"
	s.Equal(http.StatusCreated, tc.Add(toolANS))

	env.WaitForArrow(s.T(), toolANS, 120*time.Second)

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
	env.WaitForListLen(s.T(), 1, 120*time.Second)

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

	// Add the arrow that was catalogued by Follow, pinned to the fixture tag.
	toolANS := "quiver.test/quiver-test/tool-a@v1"
	s.Equal(http.StatusCreated, tc.Add(toolANS))
	env.WaitForListLen(s.T(), 1, 120*time.Second)
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
	env.WaitForListLen(s.T(), 2, 120*time.Second)
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
	env.WaitForCollectionFollowed(s.T(), gamingNS, 120*time.Second)

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

// A collection names no artifact of its own and its members carry their own refs,
// so no `version` may reach a client at either level. This reads the live JSON
// rather than the DTO struct, so a field reintroduced anywhere between the
// aggregate and the socket fails here. The seeded manifest still authors a
// `metadata.version`: the value must be discarded, not rejected, and it must not
// find its way back out.
func (s *CollectionSuite) TestGet_WireShape_CarriesNoVersion() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	c := env.Client(s.T())
	ns := "quiver.test/quiver-test/legacy-version-quiver"

	manifest := []byte(`schema: "collection@v0"
metadata:
  name: "Legacy Version Quiver"
  version: "9.9.9"
  description: "Authored before the version field was retired"
arrows:
  - namespace: quiver.test/quiver-test/tool-a@v1
`)
	s.Require().Equal(http.StatusCreated, tc.CollectionSeedManifest(ns, manifest))
	s.Require().Equal(http.StatusCreated, tc.CollectionFollow(ns))

	resp := c.CollectionGet(ns)
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode)

	var body struct {
		Data map[string]any `json:"data"`
	}
	s.Require().NoError(json.NewDecoder(resp.Body).Decode(&body))

	s.Equal("Legacy Version Quiver", body.Data["name"], "an authored version must not cost the manifest its other fields")
	s.NotContains(body.Data, "version", "the collection itself names no version")

	arrows, ok := body.Data["arrows"].([]any)
	s.Require().True(ok)
	s.Require().Len(arrows, 1)
	member, ok := arrows[0].(map[string]any)
	s.Require().True(ok)
	s.NotContains(member, "version", "a member's ref rides on its namespace")
	s.Equal("quiver.test/quiver-test/tool-a@v1", member["namespace"])
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
