//go:build integration

package advance_test

import (
	"net/http"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// A client that installed itself declares the build it runs rather than
// whatever its channel points at now, so the row offers the update that
// build actually needs, and the update moves that same row.
func (s *AdvanceSuite) TestAdopt_OlderStableMember_OffersTheUpdateAndAdvancesInPlace() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.2.0", manifest)
	v120 := s.tagCommit(f, "v1.2.0")
	v130 := s.publish(f, "v1.3.0", manifest)
	s.publish(f, "nightly", manifest)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("stable")

	s.Equal(http.StatusBadRequest, tc.Adopt(ns, "nightly"), "a pointer tag is no member of stable")
	s.Equal(http.StatusNotFound, tc.Adopt(ns, "v9.9.9"), "a ref the repository does not hold")
	items, status := tc.List()
	s.Require().Equal(http.StatusOK, status)
	s.Empty(items, "a refused adoption files nothing")

	s.Require().Equal(http.StatusCreated, tc.Adopt(ns, "v1.2.0"))
	s.Require().Equal(http.StatusCreated, tc.Adopt(ns, "v1.2.0"), "re-announcing the same build is idempotent")

	adopted := kit.WaitForDetail(s.T(), tc, ns, "v1.3.0 offered to the adopted v1.2.0", wait,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.Available != nil && d.Available.Commit == v130
		},
	)
	s.Equal(ns, adopted.Namespace)
	s.Equal("channel", adopted.SelectorKind)
	s.Equal("v1.2.0", adopted.ResolvedRef)
	s.Equal(v120, adopted.InstalledCommit)
	s.Equal(dto.AvailableDTO{Ref: "v1.3.0", Commit: v130}, *adopted.Available)
	s.True(adopted.UserInstalled)

	library, status := tc.ListUserInstalled()
	s.Require().Equal(http.StatusOK, status)
	s.Require().Len(library, 1)
	s.Require().Len(library[0].Versions, 1)
	s.Equal("stable", library[0].Versions[0].Ref)

	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, wait)
	s.update(tc, ns)
	advanced := s.waitAdvanced(tc, ns, v130)

	s.Equal(ns, advanced.Namespace, "an update keeps the adopted identity")
	s.Equal("v1.3.0", advanced.ResolvedRef)
	s.True(advanced.UserInstalled)
	s.Equal([]string{"v1.3.0"}, s.updateRuns(env, ns))

	items, status = tc.List()
	s.Require().Equal(http.StatusOK, status)
	s.Require().Len(items, 1)
	s.Require().Len(items[0].Versions, 1, "an update moves the adopted row; it never adds a successor")
}
