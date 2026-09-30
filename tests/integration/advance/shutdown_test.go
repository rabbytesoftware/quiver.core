//go:build integration

package advance_test

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

var errDaemonDied = errors.New("the daemon died mid-commit")

// commitGate holds the first live ref listing made once it is armed, the
// re-resolve an update's commit opens with, until the test lets it go or
// fails it.
type commitGate struct {
	manifold.Manifold
	mu      sync.Mutex
	armed   bool
	entered chan struct{}
	release chan error
}

func newCommitGate() *commitGate {
	return &commitGate{entered: make(chan struct{}), release: make(chan error, 1)}
}

func (g *commitGate) wrap(inner manifold.Manifold) manifold.Manifold {
	g.Manifold = inner
	return g
}

func (g *commitGate) arm() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.armed = true
}

func (g *commitGate) FreshSnapshot(ctx context.Context, ns domain.Namespace) (domain.RefSnapshot, error) {
	g.mu.Lock()
	held := g.armed
	g.armed = false
	g.mu.Unlock()
	if !held {
		return g.Manifold.FreshSnapshot(ctx, ns)
	}

	close(g.entered)
	select {
	case err := <-g.release:
		if err != nil {
			return domain.RefSnapshot{}, err
		}
	case <-ctx.Done():
		return domain.RefSnapshot{}, ctx.Err()
	}
	return g.Manifold.FreshSnapshot(ctx, ns)
}

// updateHeldInCommit installs ns at its current nightly, moves the tag and
// runs an update whose steps succeed, returning once the update's commit is
// in flight and held by gate. It returns the commit the update targets.
func (s *AdvanceSuite) updateHeldInCommit(env *kit.Env, gate *commitGate, f fixture) string {
	tc := env.TypedClient(s.T())
	ns := f.ns("nightly")

	s.install(env, tc, ns)
	moved := s.moveTag(f, "nightly")

	s.touchWorkFile(env, ns, holdFile)
	s.update(tc, ns)
	env.WaitForState(s.T(), ns, domain.ArrowStateUpdating, wait)
	gate.arm()
	s.touchWorkFile(env, ns, releaseFile)

	select {
	case <-gate.entered:
	case <-time.After(wait):
		s.FailNow("the update's commit never started")
	}
	return moved
}

// waitServerDown waits until env no longer answers requests: its shutdown has
// begun and reaches the app layer next.
func (s *AdvanceSuite) waitServerDown(env *kit.Env) {
	client := env.HTTPClient(time.Second)
	target := env.URL + "/v0/arrow/" + url.PathEscape("probe/probe/probe")
	s.Require().Eventually(func() bool {
		resp, err := client.Get(target)
		if err != nil {
			return true
		}
		_ = resp.Body.Close()
		return false
	}, wait, 5*time.Millisecond, "the server never stopped answering")
}

// A graceful shutdown waits for an update's commit in flight: the row has
// advanced by the time Close returns, and a restart finds it current.
func (s *AdvanceSuite) TestAdvance_GracefulShutdownMidCommit_LandsBeforeCloseReturns() {
	f := s.rollingFixture()
	gate := newCommitGate()
	home := s.T().TempDir()
	env := kit.BuildEnv(s.T(), s.Repos, s.CollectionRepos, home, kit.WithManifoldWrapper(gate.wrap))
	ns := f.ns("nightly")
	moved := s.updateHeldInCommit(env, gate, f)

	closed := make(chan struct{})
	go func() {
		env.Close()
		close(closed)
	}()
	s.waitServerDown(env)
	gate.release <- nil

	select {
	case <-closed:
	case <-time.After(wait):
		s.FailNow("Close never returned")
	}

	restarted := kit.BuildEnv(s.T(), s.Repos, s.CollectionRepos, home)
	detail := s.detail(restarted.TypedClient(s.T()), ns)
	s.Equal(moved, detail.InstalledCommit, "the commit landed before the stores closed")
	s.Nil(detail.Available)
	s.False(detail.Outdated)
	s.Equal([]string{"nightly"}, s.updateRuns(restarted, ns), "no update step runs again")
}

// Nothing survives a daemon that dies mid-commit, so nothing can be waited
// for. The row stays consistently outdated at what it had installed, and the
// next update runs the update steps again and advances it: the documented
// worst case is an extra update, never a missed one.
func (s *AdvanceSuite) TestAdvance_DaemonDiesMidCommit_NextUpdateRunsAgain() {
	f := s.rollingFixture()
	installed := s.tagCommit(f, "nightly")
	gate := newCommitGate()
	home := s.T().TempDir()
	env := kit.BuildEnv(s.T(), s.Repos, s.CollectionRepos, home, kit.WithManifoldWrapper(gate.wrap))
	ns := f.ns("nightly")
	moved := s.updateHeldInCommit(env, gate, f)

	env.CloseCrashing()
	gate.release <- errDaemonDied

	restarted := kit.BuildEnv(s.T(), s.Repos, s.CollectionRepos, home)
	tc := restarted.TypedClient(s.T())
	stale := s.detail(tc, ns)
	s.Equal(installed, stale.InstalledCommit, "nothing was stamped")
	s.Require().NotNil(stale.Available, "the row stays outdated")
	s.Equal(moved, stale.Available.Commit)
	s.True(stale.Outdated)

	s.update(tc, ns)
	s.waitAdvanced(tc, ns, moved)
	s.Equal([]string{"nightly", "nightly"}, s.updateRuns(restarted, ns), "the update steps ran again")
}
