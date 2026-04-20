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

// TestVersioning_UpgradeRef verifies the full v1 → v2 upgrade path when UpgradeRef: true
// is sent for a constraint-tracked arrow (glob @v*). The upgrade repo starts with only v1;
// v2 is injected mid-test so the constraint resolves to it on the update call.
func (s *IntegrationSuite) TestVersioning_UpgradeRef() {
	v1Content := readFixture(s.T(), "versioned/v1/arrow.yaml")
	v2Content := readFixture(s.T(), "versioned/v2/arrow.yaml")

	upgradeStorer := buildUpgradeRepo(s.T(), v1Content)
	s.repos["quiver-test/versioned-upgrade"] = upgradeStorer
	defer delete(s.repos, "quiver-test/versioned-upgrade")

	env := s.newEnv()
	c := env.client(s.T())

	// Glob @v* resolves to v1 — only tag present.
	ns := nsForGlob("quiver-test/versioned-upgrade", "v*")
	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	v1ns := nsFor("quiver-test/versioned-upgrade", "v1")

	resp = c.Install(v1ns, nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	// versioned v1 depends on tool-a@v1; wait for it then for v1 itself.
	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	waitForState(s.T(), c, v1ns, domain.ArrowStateReady, 30*time.Second)

	// Inject v2 into the repo — constraint @v* now resolves to v2.
	addV2ToRepo(s.T(), upgradeStorer, v2Content)

	resp = c.Update(v1ns, map[string]any{"UpgradeRef": true})
	mustStatus(s.T(), resp, http.StatusOK)

	// v2 should become ready.
	v2ns := nsFor("quiver-test/versioned-upgrade", "v2")
	waitForState(s.T(), c, v2ns, domain.ArrowStateReady, 30*time.Second)

	// v1 should be gone from the catalog.
	resp = c.GetDetail(v1ns)
	mustStatus(s.T(), resp, http.StatusNotFound)
}

// TestVersioning_AddedDepInstalledOnUpgrade verifies that when upgrading from v1 → v2,
// the new dep introduced in v2 (service-b) gets installed because InstallAdded: true.
func (s *IntegrationSuite) TestVersioning_AddedDepInstalledOnUpgrade() {
	v1Content := readFixture(s.T(), "versioned/v1/arrow.yaml")
	v2Content := readFixture(s.T(), "versioned/v2/arrow.yaml")

	upgradeStorer := buildUpgradeRepo(s.T(), v1Content)
	s.repos["quiver-test/versioned-upgrade-added"] = upgradeStorer
	defer delete(s.repos, "quiver-test/versioned-upgrade-added")

	env := s.newEnv()
	c := env.client(s.T())

	ns := nsForGlob("quiver-test/versioned-upgrade-added", "v*")
	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	v1ns := nsFor("quiver-test/versioned-upgrade-added", "v1")

	resp = c.Install(v1ns, nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	waitForState(s.T(), c, v1ns, domain.ArrowStateReady, 30*time.Second)

	// Inject v2 — it drops tool-a and adds service-b.
	addV2ToRepo(s.T(), upgradeStorer, v2Content)

	resp = c.Update(v1ns, map[string]any{"UpgradeRef": true, "InstallAdded": true})
	mustStatus(s.T(), resp, http.StatusOK)

	v2ns := nsFor("quiver-test/versioned-upgrade-added", "v2")
	waitForState(s.T(), c, v2ns, domain.ArrowStateReady, 30*time.Second)

	// service-b (added dep in v2) is a long-running service — it reaches running, not ready.
	waitForState(s.T(), c, nsFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 30*time.Second)
}

// TestVersioning_RemovedDepUninstalledOnUpgrade verifies that when upgrading from v1 → v2,
// the dep that was dropped (tool-a) gets uninstalled because UninstallOrphans: true.
func (s *IntegrationSuite) TestVersioning_RemovedDepUninstalledOnUpgrade() {
	v1Content := readFixture(s.T(), "versioned/v1/arrow.yaml")
	v2Content := readFixture(s.T(), "versioned/v2/arrow.yaml")

	upgradeStorer := buildUpgradeRepo(s.T(), v1Content)
	s.repos["quiver-test/versioned-upgrade-removed"] = upgradeStorer
	defer delete(s.repos, "quiver-test/versioned-upgrade-removed")

	env := s.newEnv()
	c := env.client(s.T())

	ns := nsForGlob("quiver-test/versioned-upgrade-removed", "v*")
	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	v1ns := nsFor("quiver-test/versioned-upgrade-removed", "v1")

	resp = c.Install(v1ns, nil)
	mustStatus(s.T(), resp, http.StatusAccepted)

	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 30*time.Second)
	waitForState(s.T(), c, v1ns, domain.ArrowStateReady, 30*time.Second)

	// Inject v2 — it drops tool-a and adds service-b.
	addV2ToRepo(s.T(), upgradeStorer, v2Content)

	resp = c.Update(v1ns, map[string]any{"UpgradeRef": true, "UninstallOrphans": true})
	mustStatus(s.T(), resp, http.StatusOK)

	v2ns := nsFor("quiver-test/versioned-upgrade-removed", "v2")
	waitForState(s.T(), c, v2ns, domain.ArrowStateReady, 30*time.Second)

	// tool-a (dropped dep from v1) must have been uninstalled.
	waitForState(s.T(), c, nsFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, 30*time.Second)
}

// TestVersioning_ManifestRefresh verifies that PATCH with no options (zero UpdateOptions)
// refreshes the manifest metadata without changing the installed ref or creating a new version.
func (s *IntegrationSuite) TestVersioning_ManifestRefresh() {
	env := s.newEnv()
	c := env.client(s.T())
	ns := nsFor("quiver-test/versioned", "v1")

	resp := c.Add(ns)
	mustStatus(s.T(), resp, http.StatusCreated)

	resp = c.Install(ns, nil)
	mustStatus(s.T(), resp, http.StatusAccepted)
	waitForState(s.T(), c, ns, domain.ArrowStateReady, 30*time.Second)

	// PATCH with empty body — updateManifest path, no ref upgrade.
	resp = c.Update(ns, map[string]any{})
	mustStatus(s.T(), resp, http.StatusOK)

	// Arrow must still be v1 and ready.
	data := s.getDetailData(c, ns)
	returnedNs, _ := data["namespace"].(string)
	s.True(
		strings.HasSuffix(returnedNs, "@v1"),
		"namespace must still be @v1 after manifest refresh, got: %s", returnedNs,
	)
	state, _ := data["state"].(string)
	s.Equal(string(domain.ArrowStateReady), state, "arrow must still be ready after manifest refresh")

	// v2 must not have been created as a side effect.
	resp = c.GetDetail(nsFor("quiver-test/versioned", "v2"))
	mustStatus(s.T(), resp, http.StatusNotFound)
}
