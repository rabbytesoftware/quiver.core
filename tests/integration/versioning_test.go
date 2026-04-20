//go:build integration

package integration_test

import (
	"net/http"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// nsForGlob constructs a glob-constrained namespace for the given fixture.
// The glob ref (e.g. "v*") is treated as a constraint by the Add handler,
// which resolves it to the latest matching tag before storing the arrow.
func nsForGlob(fixture, glob string) string {
	return "quiver.test/" + fixture + "@" + glob
}

// getDetailData fetches the arrow detail and returns the decoded "data" map.
// Fails the test if the status code is not 200.
func (s *IntegrationSuite) getDetailData(c *client, ns string) map[string]any {
	s.T().Helper()
	resp := c.GetDetail(ns)
	s.Require().Equal(http.StatusOK, resp.StatusCode, "GetDetail(%s) must return 200", ns)
	var outer map[string]any
	decodeJSON(s.T(), resp, &outer)
	data, _ := outer["data"].(map[string]any)
	return data
}

func (s *IntegrationSuite) TestVersioning_TwoVersionsCoexist() {
	env := s.newEnv()
	c := env.client(s.T())

	// Add both versions with exact refs
	resp := c.Add(nsFor("quiver-test/versioned", "v1"))
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Add(nsFor("quiver-test/versioned", "v2"))
	mustStatus(s.T(), resp, http.StatusCreated)

	// Both versions must appear in the list under the same bare namespace.
	// The list projection is asynchronous; poll until both versions are present.
	// Note: versions[].ref is InstalledRef which is empty until installed;
	// use versions[].version (manifest semver) to distinguish v1 ("1.0.0") from v2 ("2.0.0").
	var foundV1, foundV2 bool
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp = c.List()
		var outer map[string]any
		decodeJSON(s.T(), resp, &outer)
		list, _ := outer["data"].([]any)

		foundV1, foundV2 = false, false
		for _, item := range list {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			ns, _ := entry["namespace"].(string)
			if !strings.Contains(ns, "versioned") {
				continue
			}
			versions, _ := entry["versions"].([]any)
			for _, v := range versions {
				ver, ok := v.(map[string]any)
				if !ok {
					continue
				}
				version, _ := ver["version"].(string)
				switch version {
				case "1.0.0":
					foundV1 = true
				case "2.0.0":
					foundV2 = true
				}
			}
		}

		if foundV1 && foundV2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	s.True(foundV1, "v1 (1.0.0) should appear in list under versioned bare namespace")
	s.True(foundV2, "v2 (2.0.0) should appear in list under versioned bare namespace")

	// Install v1 — only v1 transitions to ready
	resp = c.Install(nsFor("quiver-test/versioned", "v1"), nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, nsFor("quiver-test/versioned", "v1"), domain.ArrowStateReady, 30*time.Second)

	// v2 is cataloged but not installed — state must still be absent
	data := s.getDetailData(c, nsFor("quiver-test/versioned", "v2"))
	state, _ := data["state"].(string)
	s.Equal(string(domain.ArrowStateAbsent), state, "v2 state must be absent when only v1 is installed")
}

func (s *IntegrationSuite) TestVersioning_VersionPinSurvivesUpdate() {
	env := s.newEnv()
	c := env.client(s.T())
	ns := nsFor("quiver-test/versioned", "v1")

	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Install(ns, nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, ns, domain.ArrowStateReady, 30*time.Second)

	// Update with UpgradeRef: false — exact-ref arrow has no constraint stored,
	// so UpgradeRef is always a no-op. Arrow stays at v1.
	resp = c.Update(ns, map[string]any{"UpgradeRef": false})
	mustStatus(s.T(), resp, http.StatusOK)

	// Verify arrow is still accessible as v1 (namespace still resolves to @v1)
	data := s.getDetailData(c, ns)
	returnedNs, _ := data["namespace"].(string)
	s.True(strings.HasSuffix(returnedNs, "@v1"), "namespace must end with @v1 after pin update, got: %s", returnedNs)

	// v2 must not have been created by the update
	resp = c.GetDetail(nsFor("quiver-test/versioned", "v2"))
	mustStatus(s.T(), resp, http.StatusNotFound)
}

func (s *IntegrationSuite) TestVersioning_UpgradeRefNoOpWhenAtLatest() {
	env := s.newEnv()
	c := env.client(s.T())

	// Add with glob constraint @v* — resolves to v2 (highest tag in the repo).
	ns := nsForGlob("quiver-test/versioned", "v*")
	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	// The resolved namespace is versioned@v2 — it must be accessible
	resolvedNs := nsFor("quiver-test/versioned", "v2")
	data := s.getDetailData(c, resolvedNs)
	returnedNs, _ := data["namespace"].(string)
	s.True(strings.HasSuffix(returnedNs, "@v2"), "glob @v* must resolve to v2, got: %s", returnedNs)

	// v1 must not have been added (we added with @v* which resolved to v2 only)
	resp = c.GetDetail(nsFor("quiver-test/versioned", "v1"))
	mustStatus(s.T(), resp, http.StatusNotFound)

	// Install v2
	resp = c.Install(resolvedNs, nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, resolvedNs, domain.ArrowStateReady, 30*time.Second)

	// Update with UpgradeRef: true.
	// Constraint "v*" resolves to v2 == installedRef "v2".
	// No upgrade needed — falls through to updateManifest.
	resp = c.Update(resolvedNs, map[string]any{"UpgradeRef": true})
	mustStatus(s.T(), resp, http.StatusOK)

	// Arrow must still be at v2 and ready
	waitForState(s.T(), c, resolvedNs, domain.ArrowStateReady, 10*time.Second)
}

func (s *IntegrationSuite) TestVersioning_BothVersionsRemovedIndependently() {
	env := s.newEnv()
	c := env.client(s.T())

	v1ns := nsFor("quiver-test/versioned", "v1")
	v2ns := nsFor("quiver-test/versioned", "v2")

	resp := c.Add(v1ns)
	mustStatus(s.T(), resp, http.StatusCreated)
	resp = c.Add(v2ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	// Remove v1 — v2 must still be accessible
	resp = c.Remove(v1ns)
	mustStatus(s.T(), resp, http.StatusOK)

	resp = c.GetDetail(v1ns)
	mustStatus(s.T(), resp, http.StatusNotFound)

	resp = c.GetDetail(v2ns)
	mustStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	// Remove v2 — both gone
	resp = c.Remove(v2ns)
	mustStatus(s.T(), resp, http.StatusOK)

	resp = c.GetDetail(v2ns)
	mustStatus(s.T(), resp, http.StatusNotFound)
}

func (s *IntegrationSuite) TestVersioning_GlobAddResolvesToLatest() {
	env := s.newEnv()
	c := env.client(s.T())

	// The versioned fixture has two tags: v1 and v2.
	// Adding with @v* constraint must resolve to v2 (descending sort picks highest).
	ns := nsForGlob("quiver-test/versioned", "v*")
	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	// v2 must be in the catalog
	resp = c.GetDetail(nsFor("quiver-test/versioned", "v2"))
	mustStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	// v1 must NOT be in the catalog (only v2 was resolved)
	resp = c.GetDetail(nsFor("quiver-test/versioned", "v1"))
	mustStatus(s.T(), resp, http.StatusNotFound)

	// The list must show versioned at v2 (version "2.0.0").
	// Note: versions[].ref is InstalledRef (empty until installed); use version semver.
	var foundV2 bool
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp = c.List()
		var outer map[string]any
		decodeJSON(s.T(), resp, &outer)
		list, _ := outer["data"].([]any)

		for _, item := range list {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			entryNs, _ := entry["namespace"].(string)
			if !strings.Contains(entryNs, "versioned") {
				continue
			}
			versions, _ := entry["versions"].([]any)
			for _, v := range versions {
				ver, ok := v.(map[string]any)
				if !ok {
					continue
				}
				version, _ := ver["version"].(string)
				if version == "2.0.0" {
					foundV2 = true
				}
			}
		}

		if foundV2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	s.True(foundV2, "glob add @v* must result in v2 (2.0.0) appearing in the list")
}
