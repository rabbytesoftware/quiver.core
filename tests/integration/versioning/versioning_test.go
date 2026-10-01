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

const wait = 120 * time.Second

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

// mutate changes a fixture repo while no resolver of the running daemon
// reads one.
func (s *VersioningSuite) mutate(fn func()) {
	s.Repos.Mutate(fn)
}

// waitAdvanced waits until the row stands at ref with nothing ahead of it and
// its runtime back at ready.
func (s *VersioningSuite) waitAdvanced(tc *kit.TypedClient, ns, ref string) dto.ArrowDetailDTO {
	return kit.WaitForDetail(s.T(), tc, ns, "the row advanced to "+ref, wait,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK &&
				d.ResolvedRef == ref &&
				d.Available == nil &&
				d.State == string(domain.ArrowStateReady)
		},
	)
}

// versionsOf lists the selectors the catalog holds rows under for a bare
// namespace containing key.
func (s *VersioningSuite) versionsOf(tc *kit.TypedClient, key string) []string {
	items, status := tc.List()
	s.Require().Equal(http.StatusOK, status)
	var refs []string
	for _, item := range items {
		if !strings.Contains(item.Namespace, key) {
			continue
		}
		for _, v := range item.Versions {
			refs = append(refs, v.Ref)
		}
	}
	return refs
}

// upgradeFixture registers a repo holding v1 of fixture under key and returns
// it, so the test can publish v2 into it.
func (s *VersioningSuite) upgradeFixture(key, fixture string) *memory.Storage {
	storer := kit.BuildUpgradeRepo(s.T(), kit.ReadFixture(s.T(), fixture+"/v1/arrow.yaml"))
	s.withUpgradeRepo(key, storer)
	return storer
}

func (s *VersioningSuite) publishV2(storer *memory.Storage, fixture string) {
	v2 := kit.ReadFixture(s.T(), fixture+"/v2/arrow.yaml")
	s.mutate(func() { kit.AddV2ToRepo(s.T(), storer, v2) })
}

// installConstraint adds and installs key@v* and waits for it and the tool it
// depends on.
func (s *VersioningSuite) installConstraint(env *kit.Env, tc *kit.TypedClient, key string) string {
	ns := kit.NSForGlob(key, "v*")
	s.Require().Equal(http.StatusCreated, tc.Add(ns))
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, wait)
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, wait)
	return ns
}

func (s *VersioningSuite) TestVersioning_TwoVersionsCoexist() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/versioned", "v1")))
	s.Equal(http.StatusCreated, tc.Add(kit.NSFor("quiver-test/versioned", "v2")))

	kit.WaitForList(
		s.T(), tc, "both v1 and v2 of quiver-test/versioned to appear in the list", wait,
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
					switch v.Ref {
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
	env.WaitForState(s.T(), kit.NSFor("quiver-test/versioned", "v1"), domain.ArrowStateReady, wait)

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
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, wait)

	s.Equal(http.StatusOK, tc.Update(ns, map[string]any{}))

	detail := s.getDetail(tc, ns)
	s.True(strings.HasSuffix(detail.Namespace, "@v1"), "namespace must end with @v1 after pin update, got: %s", detail.Namespace)

	// v2 was never added, but its fixture still resolves live — GetDetail
	// reports it as absent rather than 404ing.
	v2Detail, status := tc.GetDetail(kit.NSFor("quiver-test/versioned", "v2"))
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateAbsent), v2Detail.State)
}

// A constraint row moves to the newest matching tag in place: the identity the
// user installed stays the key, and no row is ever filed under the ref it
// resolved to.
func (s *VersioningSuite) TestVersioning_Constraint_AdvancesInPlace() {
	key := "quiver-test/versioned-upgrade"
	storer := s.upgradeFixture(key, "versioned")
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	ns := s.installConstraint(env, tc, key)
	s.publishV2(storer, "versioned")

	result, status := tc.CheckAvailable(ns)
	s.Require().Equal(http.StatusOK, status)
	s.Require().NotNil(result.Available)
	s.Equal("v2", result.Available.Ref)

	s.Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodUpdate, nil))
	advanced := s.waitAdvanced(tc, ns, "v2")

	s.Equal(ns, advanced.Namespace)
	s.Equal("constraint", advanced.SelectorKind, "the constraint survives the update")
	s.Equal([]string{"v*"}, s.versionsOf(tc, key), "no successor row appears under the resolved ref")
}

// The passive check (the first read after the TTL) finds the drift, the badge
// moves, and an update from that outdated state lands the row current.
func (s *VersioningSuite) TestVersioning_PassiveCheck_OutdatedRowUpdates() {
	key := "quiver-test/versioned-outdated-upgrade"
	storer := s.upgradeFixture(key, "versioned")
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	ns := s.installConstraint(env, tc, key)
	s.publishV2(storer, "versioned")

	s.getDetail(tc, ns)
	env.WaitForState(s.T(), ns, domain.ArrowStateOutdated, wait)

	s.Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodUpdate, nil))
	s.waitAdvanced(tc, ns, "v2")
}

func (s *VersioningSuite) TestVersioning_AddedDepInstalledOnUpdate() {
	key := "quiver-test/versioned-upgrade-added"
	storer := s.upgradeFixture(key, "versioned")
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	ns := s.installConstraint(env, tc, key)
	s.publishV2(storer, "versioned")

	s.Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodUpdate, nil))
	s.waitAdvanced(tc, ns, "v2")
	// service-b is a long-running service — reaches running, not ready
	env.WaitForState(s.T(), kit.NSFor("quiver-test/service-b", "v1"), domain.ArrowStateRunning, wait)
}

func (s *VersioningSuite) TestVersioning_RemovedDepUninstalledOnUpdate() {
	key := "quiver-test/versioned-upgrade-removed"
	storer := s.upgradeFixture(key, "versioned")
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	ns := s.installConstraint(env, tc, key)
	s.publishV2(storer, "versioned")

	s.Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodUpdate, nil))
	s.waitAdvanced(tc, ns, "v2")
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateAbsent, wait)
}

func (s *VersioningSuite) TestVersioning_UpdateLifecycleRunsAfterDepSync() {
	key := "quiver-test/versioned-upgrade-update"
	storer := s.upgradeFixture(key, "versioned-update")
	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	ns := s.installConstraint(env, tc, key)
	s.publishV2(storer, "versioned-update")

	s.Equal(http.StatusAccepted, tc.Execute(ns, domain.MethodUpdate, nil))
	detail := s.waitAdvanced(tc, ns, "v2")

	s.Require().NotNil(detail.LastReturn, "LastReturn must be set after update lifecycle ran")
	s.Equal(domain.MethodUpdate, detail.LastReturn.Method, "update lifecycle steps must have run after dep sync")
	s.Equal("success", detail.LastReturn.Outcome)
}

// Nothing retargets a row: following something else is an uninstall of one
// identity and an install of another, each its own row.
func (s *VersioningSuite) TestVersioning_SwitchingSelector_IsUninstallPlusInstall() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	v1 := kit.NSFor("quiver-test/versioned", "v1")
	v2 := kit.NSFor("quiver-test/versioned", "v2")

	s.Require().Equal(http.StatusCreated, tc.Add(v1))
	s.Require().Equal(http.StatusAccepted, tc.Install(v1, nil))
	env.WaitForState(s.T(), v1, domain.ArrowStateReady, wait)

	s.Require().Equal(http.StatusAccepted, tc.Uninstall(v1, nil))
	env.WaitForState(s.T(), v1, domain.ArrowStateAbsent, wait)
	s.Require().Equal(http.StatusOK, tc.Remove(v1))

	s.Require().Equal(http.StatusCreated, tc.Add(v2))
	s.Require().Equal(http.StatusAccepted, tc.Install(v2, nil))
	env.WaitForState(s.T(), v2, domain.ArrowStateReady, wait)

	s.Equal([]string{"v2"}, s.versionsOf(tc, "quiver-test/versioned"))
	switched := s.getDetail(tc, v2)
	s.Equal("channel", switched.SelectorKind, "a tag with no version core is its own pointer channel")
	s.Equal("v2", switched.ResolvedRef)
}

func (s *VersioningSuite) TestVersioning_ManifestRefresh() {
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := kit.NSFor("quiver-test/versioned", "v1")

	s.Equal(http.StatusCreated, tc.Add(ns))
	s.Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, wait)

	s.Equal(http.StatusOK, tc.Update(ns, map[string]any{}))

	detail := s.getDetail(tc, ns)
	s.True(strings.HasSuffix(detail.Namespace, "@v1"), "namespace must still be @v1, got: %s", detail.Namespace)
	s.Equal(string(domain.ArrowStateReady), detail.State)

	// v2 was never added, but its fixture still resolves live — GetDetail
	// reports it as absent rather than 404ing.
	v2Detail, status := tc.GetDetail(kit.NSFor("quiver-test/versioned", "v2"))
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateAbsent), v2Detail.State)
}
