//go:build integration

package versioning_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/stretchr/testify/suite"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

func TestMain(m *testing.M) { kit.Main(m) }

type VersioningSuite struct{ kit.IntegrationSuite }

func TestVersioningIntegration(t *testing.T) {
	suite.Run(t, new(VersioningSuite))
}

// getDetail fetches arrow detail and requires HTTP 200.
func (s *VersioningSuite) getDetail(tc *kit.TypedClient, ns string) dto.ArrowDetailDTO {
	s.T().Helper()
	detail, status := tc.GetDetail(ns)
	s.Require().Equal(http.StatusOK, status, "GetDetail(%s) must return 200", ns)
	return detail
}

// withUpgradeRepo registers a temporary fixture repo for the duration of the test.
func (s *VersioningSuite) withUpgradeRepo(key string, storer *memory.Storage) {
	s.Repos.Set(key, storer)
	s.T().Cleanup(func() { s.Repos.Delete(key) })
}

func (s *VersioningSuite) TestVersioning_TwoVersionsCoexist() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/versioned", "v1")))
	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/versioned", "v2")))

	kit.WaitForList(
		s.T(), tc, "both v1 and v2 of quiver-test/versioned to appear in the list", 30*time.Second,
		func(items []dto.ArrowListItemDTO, status int) bool {
			if status != http.StatusOK {
				return false
			}
			var foundV1, foundV2 bool
			for _, item := range items {
				if !strings.Contains(item.Namespace, "versioned") {
					continue
				}
				for _, v := range item.Versions {
					switch v.Version {
					case "v1":
						foundV1 = true
					case "v2":
						foundV2 = true
					}
				}
			}
			return foundV1 && foundV2
		},
	)

	s.Equal(http.StatusAccepted, tc.Install(kit.NSFor("quiver-test/versioned", "v1"), nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/versioned", "v1"), domain.ArrowStateReady, 120*time.Second)

	// v2 is cataloged but not installed — state must still be absent
	detail := s.getDetail(tc, kit.NSFor("quiver-test/versioned", "v2"))
	s.Equal(string(domain.ArrowStateAbsent), detail.State)
}

func (s *VersioningSuite) TestVersioning_VersionPinSurvivesUpdate() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/versioned", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusOK, tc.Update(ns, map[string]any{}))

	detail := s.getDetail(tc, ns)
	s.True(strings.HasSuffix(detail.Namespace, "@v1"), "namespace must end with @v1 after pin update, got: %s", detail.Namespace)

	_, status := tc.GetDetail(kit.NSFor("quiver-test/versioned", "v2"))
	s.Equal(http.StatusNotFound, status)
}

func (s *VersioningSuite) TestVersioning_UpgradeRef() {
	v1Content := kit.ReadFixture(s.T(), "versioned/v1/arrow.yaml")
	v2Content := kit.ReadFixture(s.T(), "versioned/v2/arrow.yaml")

	upgradeStorer := kit.BuildUpgradeRepo(s.T(), v1Content)
	s.withUpgradeRepo("quiver-test/versioned-upgrade", upgradeStorer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	ns := kit.NSForGlob("quiver-test/versioned-upgrade", "v*")
	s.Equal(http.StatusCreated, tc.Add(ns))

	v1ns := kit.NSFor("quiver-test/versioned-upgrade", "v1")
	s.Equal(http.StatusAccepted, tc.Install(v1ns, nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), v1ns, domain.ArrowStateReady, 120*time.Second)

	kit.AddV2ToRepo(s.T(), upgradeStorer, v2Content)

	s.Equal(http.StatusOK, tc.Update(v1ns, map[string]any{"UpgradeRef": true}))

	v2ns := kit.NSFor("quiver-test/versioned-upgrade", "v2")
	env.WaitForState(s.T(), v2ns, domain.ArrowStateOutdated, 120*time.Second)
	s.Equal(http.StatusAccepted, tc.Execute(v2ns, "_update", nil))
	env.WaitForState(s.T(), v2ns, domain.ArrowStateReady, 120*time.Second)

	_, status := tc.GetDetail(v1ns)
	s.Equal(http.StatusNotFound, status)
}

func (s *VersioningSuite) TestVersioning_AddedDepInstalledOnUpgrade() {
	v1Content := kit.ReadFixture(s.T(), "versioned/v1/arrow.yaml")
	v2Content := kit.ReadFixture(s.T(), "versioned/v2/arrow.yaml")

	upgradeStorer := kit.BuildUpgradeRepo(s.T(), v1Content)
	s.withUpgradeRepo("quiver-test/versioned-upgrade-added", upgradeStorer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	ns := kit.NSForGlob("quiver-test/versioned-upgrade-added", "v*")
	s.Equal(http.StatusCreated, tc.Add(ns))

	v1ns := kit.NSFor("quiver-test/versioned-upgrade-added", "v1")
	s.Equal(http.StatusAccepted, tc.Install(v1ns, nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), v1ns, domain.ArrowStateReady, 120*time.Second)

	kit.AddV2ToRepo(s.T(), upgradeStorer, v2Content)

	s.Equal(http.StatusOK, tc.Update(v1ns, map[string]any{"UpgradeRef": true}))

	v2ns := kit.NSFor("quiver-test/versioned-upgrade-added", "v2")
	env.WaitForState(s.T(), v2ns, domain.ArrowStateOutdated, 120*time.Second)
	s.Equal(http.StatusAccepted, tc.Execute(v2ns, "_update", nil))
	env.WaitForState(s.T(), v2ns, domain.ArrowStateReady, 120*time.Second)
	// service-b is a long-running service — reaches running, not ready
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, 120*time.Second)
}

func (s *VersioningSuite) TestVersioning_RemovedDepUninstalledOnUpgrade() {
	v1Content := kit.ReadFixture(s.T(), "versioned/v1/arrow.yaml")
	v2Content := kit.ReadFixture(s.T(), "versioned/v2/arrow.yaml")

	upgradeStorer := kit.BuildUpgradeRepo(s.T(), v1Content)
	s.withUpgradeRepo("quiver-test/versioned-upgrade-removed", upgradeStorer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	ns := kit.NSForGlob("quiver-test/versioned-upgrade-removed", "v*")
	s.Equal(http.StatusCreated, tc.Add(ns))

	v1ns := kit.NSFor("quiver-test/versioned-upgrade-removed", "v1")
	s.Equal(http.StatusAccepted, tc.Install(v1ns, nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), v1ns, domain.ArrowStateReady, 120*time.Second)

	kit.AddV2ToRepo(s.T(), upgradeStorer, v2Content)

	s.Equal(http.StatusOK, tc.Update(v1ns, map[string]any{"UpgradeRef": true}))

	v2ns := kit.NSFor("quiver-test/versioned-upgrade-removed", "v2")
	env.WaitForState(s.T(), v2ns, domain.ArrowStateOutdated, 120*time.Second)
	s.Equal(http.StatusAccepted, tc.Execute(v2ns, "_update", nil))
	env.WaitForState(s.T(), v2ns, domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, 120*time.Second)
}

func (s *VersioningSuite) TestVersioning_UpdateLifecycleRunsAfterDepSync() {
	v1Content := kit.ReadFixture(s.T(), "versioned-update/v1/arrow.yaml")
	v2Content := kit.ReadFixture(s.T(), "versioned-update/v2/arrow.yaml")

	upgradeStorer := kit.BuildUpgradeRepo(s.T(), v1Content)
	s.withUpgradeRepo("quiver-test/versioned-upgrade-update", upgradeStorer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	ns := kit.NSForGlob("quiver-test/versioned-upgrade-update", "v*")
	s.Equal(http.StatusCreated, tc.Add(ns))

	v1ns := kit.NSFor("quiver-test/versioned-upgrade-update", "v1")
	s.Equal(http.StatusAccepted, tc.Install(v1ns, nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), v1ns, domain.ArrowStateReady, 120*time.Second)

	kit.AddV2ToRepo(s.T(), upgradeStorer, v2Content)

	s.Equal(http.StatusOK, tc.Update(v1ns, map[string]any{"UpgradeRef": true}))

	v2ns := kit.NSFor("quiver-test/versioned-upgrade-update", "v2")
	env.WaitForState(s.T(), v2ns, domain.ArrowStateOutdated, 120*time.Second)
	s.Equal(http.StatusAccepted, tc.Execute(v2ns, "_update", nil))
	env.WaitForState(s.T(), v2ns, domain.ArrowStateReady, 120*time.Second)

	detail := s.getDetail(tc, v2ns)
	s.Require().NotNil(detail.LastReturn, "LastReturn must be set after update lifecycle ran")
	s.Equal(domain.MethodUpdate, detail.LastReturn.Method, "update lifecycle steps must have run after dep sync")
}

func (s *VersioningSuite) TestVersioning_ManifestRefresh() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/versioned", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, 120*time.Second)

	s.Equal(http.StatusOK, tc.Update(ns, map[string]any{}))

	detail := s.getDetail(tc, ns)
	s.True(strings.HasSuffix(detail.Namespace, "@v1"), "namespace must still be @v1, got: %s", detail.Namespace)
	s.Equal(string(domain.ArrowStateReady), detail.State)

	_, status := tc.GetDetail(kit.NSFor("quiver-test/versioned", "v2"))
	s.Equal(http.StatusNotFound, status)
}
