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
		s.T(), tc, "both v1 and v2 of quiver-test/versioned to appear in the list", 120*time.Second,
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

	// v2 was never added, but its fixture still resolves live — GetDetail
	// reports it as absent rather than 404ing.
	v2Detail, status := tc.GetDetail(kit.NSFor("quiver-test/versioned", "v2"))
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateAbsent), v2Detail.State)
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

	// The old ref was forgotten by the upgrade, but its fixture still
	// resolves live — GetDetail reports it as absent rather than 404ing.
	v1Detail, status := tc.GetDetail(v1ns)
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateAbsent), v1Detail.State)
}

// TestVersioning_UpgradeRef_FromVersionOutdated proves UpgradeRef still
// installs the new ref when the old arrow is Outdated from a genuine
// version-drift detection, not only from Ready as TestVersioning_UpgradeRef
// covers.
func (s *VersioningSuite) TestVersioning_UpgradeRef_FromVersionOutdated() {
	v1Content := kit.ReadFixture(s.T(), "versioned/v1/arrow.yaml")
	v2Content := kit.ReadFixture(s.T(), "versioned/v2/arrow.yaml")

	upgradeStorer := kit.BuildUpgradeRepo(s.T(), v1Content)
	s.withUpgradeRepo("quiver-test/versioned-outdated-upgrade", upgradeStorer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	ns := kit.NSForGlob("quiver-test/versioned-outdated-upgrade", "v*")
	s.Equal(http.StatusCreated, tc.Add(ns))

	v1ns := kit.NSFor("quiver-test/versioned-outdated-upgrade", "v1")
	s.Equal(http.StatusAccepted, tc.Install(v1ns, nil))
	env.WaitForState(s.T(), kit.NSFor("quiver-test/tool-a", "v1"), domain.ArrowStateReady, 120*time.Second)
	env.WaitForState(s.T(), v1ns, domain.ArrowStateReady, 120*time.Second)

	kit.AddV2ToRepo(s.T(), upgradeStorer, v2Content)

	s.getDetail(tc, string(v1ns))
	env.WaitForState(s.T(), v1ns, domain.ArrowStateOutdated, 120*time.Second)

	s.Equal(http.StatusOK, tc.Update(v1ns, map[string]any{"UpgradeRef": true}))

	v2ns := kit.NSFor("quiver-test/versioned-outdated-upgrade", "v2")
	env.WaitForState(s.T(), v2ns, domain.ArrowStateOutdated, 120*time.Second)
	s.Equal(http.StatusAccepted, tc.Execute(v2ns, "_update", nil))
	env.WaitForState(s.T(), v2ns, domain.ArrowStateReady, 120*time.Second)
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

// TestVersioning_SwitchChannel proves PATCH /v0/arrow/:ns's channel field end
// to end under the architecture this round's fix moved it to: picking a
// channel never upgrades inline any more. It only records the preference
// (SetChannel) and kicks off an immediate, asynchronous version check —
// the very same outdated-detection + explicit-Update flow every other
// arrow already goes through does the actual work, not a channel-specific
// special case.
func (s *VersioningSuite) TestVersioning_SwitchChannel() {
	key := "quiver-test/channel-switch"
	storer := kit.BuildBranchOnlyRepo(s.T(), kit.BuildMinimalYAML("initial commit"))
	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.0.0", kit.BuildMinimalYAML("v1.0.0 content"))
	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.1.0-beta.1", kit.BuildMinimalYAML("v1.1.0-beta.1 content"))
	s.withUpgradeRepo(key, storer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	stableNs := kit.NSFor(key, "v1.0.0")
	s.Equal(http.StatusCreated, tc.Add(stableNs))
	s.Equal(http.StatusAccepted, tc.Install(stableNs, nil))
	env.WaitForState(s.T(), stableNs, domain.ArrowStateReady, 120*time.Second)

	stableDetail := s.getDetail(tc, stableNs)
	s.Equal("stable", stableDetail.Channel, "an exact v1.0.0 ref must classify onto the stable channel")

	// The PATCH itself must return fast: no live git resolve in the response
	// path, only the durable channel write.
	start := time.Now()
	s.Equal(http.StatusOK, tc.Update(stableNs, map[string]any{"Channel": "beta"}))
	s.Less(time.Since(start), 2*time.Second,
		"switching channel must return immediately, not block on a live version check")

	// Still installed at v1.0.0 right after the PATCH returns — no inline
	// upgrade happened. The async check then lands Outdated/RecommendedRef
	// once it resolves the beta channel's latest against the still-installed
	// ref, the same badge any other outdated arrow gets.
	afterSwitch := kit.WaitForDetail(
		s.T(), tc, stableNs, "outdated=true, recommended_ref=v1.1.0-beta.1", 120*time.Second,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.Outdated && d.RecommendedRef == "v1.1.0-beta.1"
		},
	)
	s.Equal("beta", afterSwitch.Channel, "the channel switch must reach the read model even though no upgrade ran")
	// The runtime itself legitimately flips Ready -> Outdated the moment the
	// drift lands (the same runtime-state sync every other outdated arrow
	// gets) -- confirming that, not just the catalog's Outdated flag, is
	// what proves the async check reached all the way through.
	s.Equal(string(domain.ArrowStateOutdated), afterSwitch.State,
		"the async check must flip the runtime to outdated, unmoved from the old ref")

	// Only now, an explicit Update click performs the real upgrade — this is
	// the widened Update gate: a channel-tracked arrow with no
	// InstalledConstraint at all still reaches upgradeRef.
	s.Equal(http.StatusOK, tc.Update(stableNs, map[string]any{"UpgradeRef": true}))

	// This fixture carries no dependencies between v1.0.0 and v1.1.0-beta.1,
	// so onArrowUpgraded's own dependency-diff branch (see
	// usecases/runtime.go) takes its "no diff" arm and reinstalls the new
	// row directly, reaching Ready without an intermediate Outdated stop —
	// unlike TestVersioning_UpgradeRef's fixture, which does carry a
	// dependency change and stops at Outdated pending an explicit _update.
	betaNs := kit.NSFor(key, "v1.1.0-beta.1")
	env.WaitForState(s.T(), betaNs, domain.ArrowStateReady, 120*time.Second)

	betaDetail := s.getDetail(tc, betaNs)
	s.Equal("beta", betaDetail.Channel, "the tracked channel must carry forward onto the upgraded row")
	s.True(strings.HasSuffix(betaDetail.Namespace, "@v1.1.0-beta.1"),
		"namespace must end with the beta channel's ref, got: %s", betaDetail.Namespace)

	// The old ref was forgotten by the upgrade, but its fixture still
	// resolves live — GetDetail reports it as absent rather than 404ing.
	oldDetail, status := tc.GetDetail(stableNs)
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateAbsent), oldDetail.State)
}

// TestVersioning_SwitchChannel_GlobInstalledArrow_ClearsStaleConstraint is
// the regression a review caught in commit 45d8fc76: a glob install
// (resolveGlob) leaves an arrow with BOTH InstalledConstraint set AND a
// classified Channel. ResolveTrackedRef is constraint-first, so without
// SetChannel clearing the constraint, switching channel on such an arrow
// would have zero effect on drift-check resolution ever again — exactly
// the "dropdown does nothing" bug class this whole feature exists to kill,
// recurring in a narrower case. End to end over the real HTTP API: install
// via a glob (carrying both a constraint and a channel), switch to a
// different channel, and confirm drift detection actually picks up the new
// channel's target rather than silently resolving back through the
// leftover constraint to the ref already installed.
func (s *VersioningSuite) TestVersioning_SwitchChannel_GlobInstalledArrow_ClearsStaleConstraint() {
	key := "quiver-test/channel-switch-glob"
	storer := kit.BuildBranchOnlyRepo(s.T(), kit.BuildMinimalYAML("initial commit"))
	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.0.0", kit.BuildMinimalYAML("v1.0.0 content"))
	kit.AddTaggedCommitToRepo(s.T(), storer, "v1.1.0-beta.1", kit.BuildMinimalYAML("v1.1.0-beta.1 content"))
	s.withUpgradeRepo(key, storer)

	env := s.NewEnv()
	tc := env.TypedClient(s.T())

	// "v1.0.*" resolves to exactly v1.0.0 (v1.1.0-beta.1 does not match the
	// prefix at all, sidestepping any ambiguity over whether a plain "*"
	// wildcard would rank a pre-release tag ahead of it), landing an arrow
	// with BOTH InstalledConstraint="v1.0.*" AND a classified Channel=
	// "stable" -- the exact glob-install shape the regression needed.
	globNs := kit.NSForGlob(key, "v1.0.*")
	s.Equal(http.StatusCreated, tc.Add(globNs))

	installedNs := kit.NSFor(key, "v1.0.0")
	s.Equal(http.StatusAccepted, tc.Install(installedNs, nil))
	env.WaitForState(s.T(), installedNs, domain.ArrowStateReady, 120*time.Second)

	installedDetail := s.getDetail(tc, installedNs)
	s.Equal("stable", installedDetail.Channel)
	s.NotEmpty(installedDetail.InstalledConstraint, "a glob install must leave an InstalledConstraint behind")

	// Switch to "beta", whose only member (v1.1.0-beta.1) is a different
	// ref entirely from what is installed -- deliberately, so a false "not
	// outdated" caused by the regression (ResolveTrackedRef falling back
	// to the leftover "v1.0.*" constraint, which always re-resolves to the
	// ref already installed) cannot be mistaken for a fixed channel-aware
	// resolution by coincidence.
	s.Equal(http.StatusOK, tc.Update(installedNs, map[string]any{"Channel": "beta"}))

	afterSwitch := kit.WaitForDetail(
		s.T(), tc, installedNs, "outdated=true, recommended_ref=v1.1.0-beta.1", 120*time.Second,
		func(d dto.ArrowDetailDTO, status int) bool {
			return status == http.StatusOK && d.Outdated && d.RecommendedRef == "v1.1.0-beta.1"
		},
	)
	s.Equal("beta", afterSwitch.Channel)
	s.Empty(afterSwitch.InstalledConstraint, "the channel switch must clear the leftover constraint")
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

	// v2 was never added, but its fixture still resolves live — GetDetail
	// reports it as absent rather than 404ing.
	v2Detail, status := tc.GetDetail(kit.NSFor("quiver-test/versioned", "v2"))
	s.Equal(http.StatusOK, status)
	s.Equal(string(domain.ArrowStateAbsent), v2Detail.State)
}
