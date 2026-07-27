//go:build integration

package concurrency_test

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
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

	env.WaitForListLen(s.T(), 1, 120*time.Second)
	items, _ := tc.List()
	s.Len(items, 1, "concurrent adds of the same namespace should result in exactly one catalog entry")
}

func (s *ConcurrencySuite) TestConcurrency_ConcurrentInstallsSharedDep() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/composed-c", "v1")))
	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/composed-c", "v1"), nil))

	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 120*time.Second)

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

	// Whichever of the two racing requests wins, the runtime must settle. The
	// waiter carries that assertion: a state stuck in installing or uninstalling
	// never satisfies it and fails the test.
	kit.WaitForDetail(
		s.T(), tc, ns,
		"a terminal state (ready or absent) after concurrent install+uninstall",
		60*time.Second,
		func(detail dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK &&
				(detail.State == string(domain.ArrowStateReady) ||
					detail.State == string(domain.ArrowStateAbsent))
		},
	)
}
