//go:build integration

package concurrency_test

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/tests/integration/kit"
	"github.com/stretchr/testify/suite"
)

func TestMain(m *testing.M) { kit.Main(m) }

type ConcurrencySuite struct{ kit.IntegrationSuite }

func TestConcurrencyIntegration(t *testing.T) {
	suite.Run(t, new(ConcurrencySuite))
}

func (s *ConcurrencySuite) TestConcurrency_AddSameNamespace() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/tool-a", "v1")

	const N = 10
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tc.Add(ns)
		}()
	}
	wg.Wait()

	items := kit.WaitForListLen(s.T(), tc, 1, 10*time.Second)
	s.Len(items, 1, "concurrent adds of the same namespace should result in exactly one catalog entry")
}

func (s *ConcurrencySuite) TestConcurrency_ConcurrentInstallsSharedDep() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/composed-c", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/composed-c", "v1"), nil))

	kit.WaitForState(s.T(), tc, kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	kit.WaitForState(s.T(), tc, kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 30*time.Second)
	kit.WaitForState(s.T(), tc, kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 30*time.Second)

	// A second install while already ready is idempotent — returns 202.
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/composed-c", "v1"), nil),
		"second install of an already-ready arrow must be idempotent (202)")
}

func (s *ConcurrencySuite) TestConcurrency_ConcurrentListUnderLoad() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 5; i++ {
			tc.Add(kit.NSFor("quiver-test/tool-a", "v1"))
		}
	}()

	const N = 50
	errs := make([]bool, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, status := tc.List()
			if status != http.StatusOK {
				errs[idx] = true
			}
		}(i)
	}
	wg.Wait()
	<-done

	for i, hadErr := range errs {
		s.Falsef(hadErr, "goroutine %d: list request failed", i)
	}
}

func (s *ConcurrencySuite) TestConcurrency_InstallAndUninstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	c := env.Client(s.T()) // raw client for fire-and-forget goroutine
	ns := kit.NSFor("quiver-test/tool-a-slow", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp := c.Install(ns, nil)
		resp.Body.Close()
	}()

	// Immediately attempt uninstall — races with install
	resp := c.Uninstall(ns, nil)
	resp.Body.Close()

	wg.Wait()

	// Final state must be terminal: ready or absent, never stuck
	deadline := time.Now().Add(20 * time.Second)
	var finalState domain.ArrowState
	for time.Now().Before(deadline) {
		detail, status := tc.GetDetail(ns)
		if status == http.StatusOK {
			finalState = domain.ArrowState(detail.State)
			if finalState == domain.ArrowStateReady || finalState == domain.ArrowStateAbsent {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	s.True(
		finalState == domain.ArrowStateReady || finalState == domain.ArrowStateAbsent,
		"after concurrent install+uninstall, state must be terminal (ready or absent), got: %s",
		finalState,
	)
}
