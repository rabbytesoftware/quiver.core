//go:build integration

package quivers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/rabbytesoftware/quiver/tests/integration/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type QuiverSuite struct{ kit.IntegrationSuite }

func TestQuiverIntegration(t *testing.T) {
	suite.Run(t, new(QuiverSuite))
}

func (s *QuiverSuite) TestFollow_And_Get() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.QuiverNSFor("gaming-quiver", "v1")

	// Pre-seed the tool-a arrow so it's in the catalog before following the quiver.
	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/tool-a", "v1")))

	s.Equal(http.StatusCreated, tc.QuiverFollow(ns))

	detail, status := tc.QuiverGet(ns)
	s.Equal(http.StatusOK, status)
	s.Equal("Gaming Quiver", detail.Name)
	s.Len(detail.Arrows, 2)
	s.True(detail.Followed)
}

func (s *QuiverSuite) TestGet_Uncached() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.QuiverNSFor("yaml-quiver", "v1")

	detail, status := tc.QuiverGet(ns)
	s.Equal(http.StatusOK, status)
	s.Equal("YAML Quiver", detail.Name)
	s.False(detail.Followed)
}

func (s *QuiverSuite) TestList_FollowedFilter() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	gamingNS := kit.QuiverNSFor("gaming-quiver", "v1")

	s.Equal(http.StatusCreated, tc.QuiverFollow(gamingNS))

	trueVal := true
	followed, status := tc.QuiverList(&trueVal)
	s.Equal(http.StatusOK, status)
	s.Len(followed, 1)
	s.Equal(gamingNS, followed[0].Namespace)

	falseVal := false
	unfollowed, status := tc.QuiverList(&falseVal)
	s.Equal(http.StatusOK, status)
	for _, q := range unfollowed {
		s.NotEqual(gamingNS, q.Namespace)
	}

	all, status := tc.QuiverList(nil)
	s.Equal(http.StatusOK, status)
	found := false
	for _, q := range all {
		if q.Namespace == gamingNS {
			found = true
			break
		}
	}
	s.True(found, "gaming-quiver should appear in all-quivers list")
}

func (s *QuiverSuite) TestFollow_Duplicate_Returns409() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.QuiverNSFor("yaml-quiver", "v1")

	s.Equal(http.StatusCreated, tc.QuiverFollow(ns))
	s.Equal(http.StatusConflict, tc.QuiverFollow(ns))
}

func (s *QuiverSuite) TestUnfollow_NotFollowed_Returns404() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.QuiverNSFor("gaming-quiver", "v1")

	s.Equal(http.StatusNotFound, tc.QuiverUnfollow(ns))
}

func (s *QuiverSuite) TestPartialArrowFailure() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.QuiverNSFor("missing-arrow-quiver", "v1")

	s.Equal(http.StatusCreated, tc.QuiverFollow(ns))

	detail, status := tc.QuiverGet(ns)
	s.Equal(http.StatusOK, status)
	s.Require().Len(detail.Arrows, 1)
	s.False(detail.Arrows[0].Resolved)
}

func (s *QuiverSuite) TestSeedManifest_ThenGet() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := "quiver.test/quiver-test/seeded-quiver"

	body := []byte(`schema: "quiver@v0"
metadata:
  name: "Seeded Quiver"
  description: "A seeded quiver"
arrows:
  - namespace: quiver.test/quiver-test/tool-a
`)
	resp := env.Client(s.T()).QuiverSeedManifest(ns, body)
	kit.MustStatus(s.T(), resp, http.StatusCreated)

	detail, status := tc.QuiverGet(ns)
	s.Equal(http.StatusOK, status)
	s.Equal("Seeded Quiver", detail.Name)
}

func (s *QuiverSuite) TestValidateManifest_InvalidSchema() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	result, status := tc.QuiverValidateManifest("quiver.test/quiver-test/any-quiver", []byte(`{}`))
	s.Equal(http.StatusUnprocessableEntity, status)
	s.False(result.Valid)
	s.NotEmpty(result.Errors)
}
