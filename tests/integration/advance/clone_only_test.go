//go:build integration

package advance_test

import (
	"net/http"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// A self-hosted git server is reachable only by cloning, which checks out a
// tag or a branch but never a commit SHA; no selector names a git ref there
// either, so every fetch must name the ref the selector resolved to.
func (s *AdvanceSuite) TestAdvance_CloneOnlyHost_StableChannel_InstallsAndUpdatesAcrossANewTag() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "stable-26.5.0", manifest)
	s.publish(f, "stable-26.5.1", manifest)
	installed := s.publish(f, "stable-26.6.0", manifest)
	env := s.NewEnv(kit.WithCloneOnlyHost())
	tc := env.TypedClient(s.T())
	ns := f.ns("stable")

	s.Require().Equal(http.StatusCreated, tc.Add("quiver.test/"+f.key), "a refless add follows the default channel")
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, wait)

	detail := s.detail(tc, ns)
	s.Equal(ns, detail.Namespace)
	s.Equal("channel", detail.SelectorKind)
	s.Equal("stable-26.6.0", detail.ResolvedRef)
	s.Equal(installed, detail.InstalledCommit)

	target := s.publish(f, "stable-26.7.0", manifest)
	poller := s.pollDetail(env, ns)

	available := s.checkAvailable(tc, ns)
	s.Require().NotNil(available)
	s.Equal(dto.AvailableDTO{Ref: "stable-26.7.0", Commit: target}, *available)

	s.update(tc, ns)
	advanced := s.waitAdvanced(tc, ns, target)

	polls, bad := poller.stop()
	s.Positive(polls)
	s.Empty(bad, "%s must stay resolvable while it moves to stable-26.7.0", ns)
	s.Equal(ns, advanced.Namespace, "an update keeps the identity")
	s.Equal("stable-26.7.0", advanced.ResolvedRef)
	s.Equal([]string{"stable-26.7.0"}, s.updateRuns(env, ns), "${REF} is the target tag, never the channel name")
}

func (s *AdvanceSuite) TestAdvance_CloneOnlyHost_Constraint_InstallsAndUpdatesWithinBounds() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.2.0", manifest)
	installed := s.publish(f, "v1.3.0", manifest)
	s.publish(f, "v2.0.0", manifest)
	env := s.NewEnv(kit.WithCloneOnlyHost())
	tc := env.TypedClient(s.T())
	ns := kit.NSForGlob(f.key, "v1.*")

	s.install(env, tc, ns)
	detail := s.detail(tc, ns)
	s.Equal("constraint", detail.SelectorKind)
	s.Equal("v1.3.0", detail.ResolvedRef)
	s.Equal(installed, detail.InstalledCommit)

	target := s.publish(f, "v1.4.0", manifest)
	s.update(tc, ns)
	advanced := s.waitAdvanced(tc, ns, target)

	s.Equal(ns, advanced.Namespace)
	s.Equal("v1.4.0", advanced.ResolvedRef)
	s.Equal([]string{"v1.4.0"}, s.updateRuns(env, ns))
}

// Adopting on such a host fetches the declared build at its own tag.
func (s *AdvanceSuite) TestAdopt_CloneOnlyHost_OlderStableMember_OffersTheUpdate() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "stable-26.5.0", manifest)
	adopted := s.tagCommit(f, "stable-26.5.0")
	newest := s.publish(f, "stable-26.6.0", manifest)
	env := s.NewEnv(kit.WithCloneOnlyHost())
	tc := env.TypedClient(s.T())
	ns := f.ns("stable")

	s.Require().Equal(http.StatusCreated, tc.Adopt(ns, "stable-26.5.0"))

	detail := kit.WaitForDetail(s.T(), tc, ns, "stable-26.6.0 offered to the adopted stable-26.5.0", wait,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.Available != nil && d.Available.Commit == newest
		},
	)
	s.Equal("stable-26.5.0", detail.ResolvedRef)
	s.Equal(adopted, detail.InstalledCommit)
	s.Equal(dto.AvailableDTO{Ref: "stable-26.6.0", Commit: newest}, *detail.Available)
}
