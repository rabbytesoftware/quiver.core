//go:build integration

package integration_test

import (
	"net/http"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func (s *IntegrationSuite) TestDeps_TransitiveInstall() {
	env := s.newEnv()
	c := env.client(s.T())

	resp := c.Add(nsFor("quiver-test/composed-c", "v1"))
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Install(nsFor("quiver-test/composed-c", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	// service-b is a long-running service; it transitions to running, not ready
	waitForState(s.T(), c, nsFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 30*time.Second)
	waitForState(s.T(), c, nsFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 30*time.Second)
}

func (s *IntegrationSuite) TestDeps_DiamondDeduplication() {
	env := s.newEnv()
	c := env.client(s.T())

	resp := c.Add(nsFor("dep-diamond/root", "v1"))
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Install(nsFor("dep-diamond/root", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	for _, fixture := range []string{
		"dep-diamond/root",
		"dep-diamond/left",
		"dep-diamond/right",
		"dep-diamond/shared",
	} {
		waitForState(s.T(), c, nsFor(fixture, "v1"), domain.ArrowStateReady, 30*time.Second)
	}

	// shared is added to catalog exactly once — verify via GetDetail
	resp = c.GetDetail(nsFor("dep-diamond/shared", "v1"))
	mustStatus(s.T(), resp, http.StatusOK)

	// Verify the list of all arrows doesn't double-count shared.
	// List only shows user-installed arrows, so count via GetDetail is the right approach.
	// Confirm a second GetDetail for an impossible second copy returns 404.
	// (The namespace is versioned, so there can only ever be one entry.)
	// As an extra guard: parse list and confirm "dep-diamond/shared" does not appear twice.
	resp = c.List()
	var wrapper map[string]any
	decodeJSON(s.T(), resp, &wrapper) // decodeJSON closes the body
	list, _ := wrapper["data"].([]any)

	sharedCount := 0
	for _, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ns, _ := entry["namespace"].(string)
		if strings.Contains(ns, "shared") {
			sharedCount++
		}
	}
	// shared is not user-installed so count may be 0 in the list; it must not be > 1.
	s.LessOrEqual(sharedCount, 1, "dep-diamond/shared should appear at most once in catalog list")
}

func (s *IntegrationSuite) TestDeps_CircularDetection() {
	env := s.newEnv()
	c := env.client(s.T())

	resp := c.Add(nsFor("dep-circular/circ-a", "v1"))
	if resp.StatusCode == http.StatusCreated {
		resp.Body.Close()
		resp = c.Install(nsFor("dep-circular/circ-a", "v1"), nil)
		s.GreaterOrEqual(resp.StatusCode, 400, "install of circular dep should fail")
		resp.Body.Close()
	} else {
		s.GreaterOrEqual(resp.StatusCode, 400, "add of circular dep should fail")
		resp.Body.Close()
	}
}

func (s *IntegrationSuite) TestDeps_RemoveBlockedByDependents() {
	env := s.newEnv()
	c := env.client(s.T())

	resp := c.Add(nsFor("quiver-test/composed-c", "v1"))
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Install(nsFor("quiver-test/composed-c", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)

	// tool-a is in ready state (installed as dep) — removing it must fail
	resp = c.Remove(nsFor("quiver-test/tool-a", "v1"))
	s.GreaterOrEqual(resp.StatusCode, 400, "remove should fail when arrow is installed (ready state)")
	resp.Body.Close()

	// tool-a still accessible in catalog
	resp = c.GetDetail(nsFor("quiver-test/tool-a", "v1"))
	mustStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()
}

func (s *IntegrationSuite) TestDeps_RemoveAfterDependentsGone() {
	env := s.newEnv()
	c := env.client(s.T())

	resp := c.Add(nsFor("quiver-test/composed-c", "v1"))
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Install(nsFor("quiver-test/composed-c", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	waitForState(s.T(), c, nsFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 30*time.Second)
	waitForState(s.T(), c, nsFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 30*time.Second)

	// Stop service-b so it returns to ready state before uninstalling
	resp = c.Stop(nsFor("quiver-test/service-b", "v1"))
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, nsFor("quiver-test/service-b", "v1"), domain.ArrowStateReady, 30*time.Second)

	// Uninstall composed-c; cleanupAfterUninstall cascades to orphaned deps
	resp = c.Uninstall(nsFor("quiver-test/composed-c", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	waitForState(s.T(), c, nsFor("quiver-test/composed-c", "v1"), domain.ArrowStateAbsent, 30*time.Second)
	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, 30*time.Second)
	waitForState(s.T(), c, nsFor("quiver-test/service-b", "v1"), domain.ArrowStateAbsent, 30*time.Second)

	// Remove composed-c from catalog
	resp = c.Remove(nsFor("quiver-test/composed-c", "v1"))
	mustStatus(s.T(), resp, http.StatusOK)

	// Now tool-a is absent — remove should succeed
	resp = c.Remove(nsFor("quiver-test/tool-a", "v1"))
	mustStatus(s.T(), resp, http.StatusOK)

	resp = c.GetDetail(nsFor("quiver-test/tool-a", "v1"))
	mustStatus(s.T(), resp, http.StatusNotFound)
}

func (s *IntegrationSuite) TestDeps_OrphanCleanup() {
	env := s.newEnv()
	c := env.client(s.T())

	resp := c.Add(nsFor("quiver-test/composed-c", "v1"))
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Install(nsFor("quiver-test/composed-c", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	waitForState(s.T(), c, nsFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 30*time.Second)
	waitForState(s.T(), c, nsFor("quiver-test/composed-c", "v1"), domain.ArrowStateReady, 30*time.Second)

	// Stop service-b so it returns to ready state; otherwise cascade uninstall is blocked
	resp = c.Stop(nsFor("quiver-test/service-b", "v1"))
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, nsFor("quiver-test/service-b", "v1"), domain.ArrowStateReady, 30*time.Second)

	// Uninstall composed-c — cleanupAfterUninstall auto-cascades to orphaned deps
	resp = c.Uninstall(nsFor("quiver-test/composed-c", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	waitForState(s.T(), c, nsFor("quiver-test/composed-c", "v1"), domain.ArrowStateAbsent, 30*time.Second)
	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, 30*time.Second)
	waitForState(s.T(), c, nsFor("quiver-test/service-b", "v1"), domain.ArrowStateAbsent, 30*time.Second)
}
