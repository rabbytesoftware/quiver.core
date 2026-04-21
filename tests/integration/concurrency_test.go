//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (s *IntegrationSuite) TestConcurrency_AddSameNamespace() {
	env := s.newEnv()
	c := env.client(s.T())
	ns := nsFor("quiver-test/tool-a", "v1")

	const N = 10
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := c.Add(ns)
			resp.Body.Close()
		}()
	}
	wg.Wait()

	// Catalog should have exactly one entry — projection is async, poll until stable
	list := waitForListLen(s.T(), c, 1, 10*time.Second)
	s.Len(list, 1, "concurrent adds of the same namespace should result in exactly one catalog entry")
}

func (s *IntegrationSuite) TestConcurrency_ConcurrentInstallsSharedDep() {
	env := s.newEnv()
	c := env.client(s.T())

	// Add composed-c (depends on tool-a and service-b)
	resp := c.Add(nsFor("quiver-test/composed-c", "v1"))
	mustStatus(s.T(), resp, http.StatusCreated)

	// Fire two concurrent installs of the same arrow — at least one must be accepted (202),
	// both must be either accepted (202) or rejected (4xx); never a server error (5xx).
	statuses := make([]int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r := c.Install(nsFor("quiver-test/composed-c", "v1"), nil)
			statuses[idx] = r.StatusCode
			r.Body.Close()
		}(i)
	}
	wg.Wait()

	acceptedCount := 0
	for _, code := range statuses {
		s.True(
			code == http.StatusAccepted || code >= 400 && code < 500,
			"concurrent install must be either accepted (202) or rejected (4xx), got: %d", code,
		)
		if code == http.StatusAccepted {
			acceptedCount++
		}
	}
	s.GreaterOrEqual(acceptedCount, 1, "at least one concurrent install must be accepted (202)")

	// tool-a (shared dep) and composed-c should reach ready exactly once (not corrupted/stuck)
	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	waitForState(s.T(), c, nsFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 30*time.Second)
}

func (s *IntegrationSuite) TestConcurrency_ConcurrentListUnderLoad() {
	env := s.newEnv()
	c := env.client(s.T())

	// Start background goroutine doing adds
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 5; i++ {
			resp := c.Add(nsFor("quiver-test/tool-a", "v1"))
			resp.Body.Close()
		}
	}()

	// Fire 50 concurrent list requests
	const N = 50
	errors := make([]bool, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp := c.List()
			if resp.StatusCode != http.StatusOK {
				errors[idx] = true
				resp.Body.Close()
				return
			}
			var wrapper map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
				errors[idx] = true
			}
			resp.Body.Close()
		}(i)
	}
	wg.Wait()
	<-done

	for i, hadErr := range errors {
		s.Falsef(hadErr, "goroutine %d: list request failed or returned invalid JSON", i)
	}
}

func (s *IntegrationSuite) TestConcurrency_InstallAndUninstall() {
	env := s.newEnv()
	c := env.client(s.T())
	ns := nsFor("quiver-test/tool-a-slow", "v1")

	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	// Start install
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp := c.Install(ns, nil)
		resp.Body.Close()
	}()

	// Immediately attempt uninstall — races with install
	resp = c.Uninstall(ns, nil)
	resp.Body.Close()

	wg.Wait()

	// Final state must be consistent: ready or absent, never stuck
	// Poll for a terminal state
	deadline := time.Now().Add(20 * time.Second)
	var finalState domain.ArrowState
	for time.Now().Before(deadline) {
		resp := c.GetDetail(ns)
		if resp.StatusCode == http.StatusOK {
			var outer map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&outer); err == nil {
				if data, ok := outer["data"].(map[string]any); ok {
					if st, ok := data["state"].(string); ok {
						finalState = domain.ArrowState(st)
						if finalState == domain.ArrowStateReady || finalState == domain.ArrowStateAbsent {
							break
						}
					}
				}
			} else {
				resp.Body.Close()
			}
		} else {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}

	s.True(
		finalState == domain.ArrowStateReady || finalState == domain.ArrowStateAbsent,
		"after concurrent install+uninstall, state must be terminal (ready or absent), got: %s",
		finalState,
	)
}
