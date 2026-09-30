//go:build integration

package advance_test

import (
	"slices"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// finalStates waits until the runtime stream's last state for ns is want,
// and returns every state it reported.
func (s *AdvanceSuite) finalStates(env interface{ StateHistory(string) []string }, ns string, want domain.ArrowState) []string {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		history := env.StateHistory(ns)
		if len(history) > 0 && history[len(history)-1] == string(want) {
			return history
		}
		time.Sleep(10 * time.Millisecond) // poll interval, not a race fix
	}
	s.FailNow("the runtime never reported its final state", "%s: want %s, got %v", ns, want, env.StateHistory(ns))
	return nil
}

// A committed update goes updating -> ready. The badge is the row's: after
// the steps end the row still names the target as available until its
// commit lands, and the runtime must not read that as outdated in between.
func (s *AdvanceSuite) TestAdvance_CommittedUpdate_BadgeNeverFlickersOutdated() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.2.0", manifest)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("stable")

	s.install(env, tc, ns)
	v130 := s.publish(f, "v1.3.0", manifest)
	s.update(tc, ns)
	s.waitAdvanced(tc, ns, v130)
	s.runtimeSettled(env, ns)

	history := s.finalStates(env, ns, domain.ArrowStateReady)
	began := slices.Index(history, string(domain.ArrowStateUpdating))
	s.Require().GreaterOrEqual(began, 0, "the update ran: %v", history)
	s.NotContains(history[began:], string(domain.ArrowStateOutdated),
		"after the update began the badge must never read outdated: %v", history)
}
