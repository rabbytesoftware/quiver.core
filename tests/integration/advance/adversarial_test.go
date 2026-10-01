//go:build integration

package advance_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/tests/kit"
)

// runtimeSettled waits until GET /v0/runtime/:ns reports the row settled: no
// update bracket open and no commit in flight, so the row reads final.
func (s *AdvanceSuite) runtimeSettled(env *kit.Env, ns string) {
	client := env.HTTPClient(30 * time.Second)
	target := env.URL + "/v0/runtime/" + url.PathEscape(ns)
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		resp, err := client.Get(target)
		s.Require().NoError(err)
		var body struct {
			Data dto.ArrowRuntimeDTO `json:"data"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		s.Require().NoError(decodeErr)
		active := body.Data.State == string(domain.ArrowStateUpdating) || body.Data.State == string(domain.ArrowStateStopping)
		if resp.StatusCode == http.StatusOK && !body.Data.Settling && !active {
			return
		}
		time.Sleep(10 * time.Millisecond) // poll interval, not a race fix
	}
	s.FailNow("the row never settled", ns)
}

// noUpdateStepsManifest has install steps only: an update re-runs them in
// place. The install records ${REF}; fail makes the install exit non-zero.
func noUpdateStepsManifest(fail bool) []byte {
	failCmd := ""
	if fail {
		failCmd = "; exit 3"
	}
	return []byte(`schema: "arrow@v0"
metadata:
  name: quiver-test.no-update-steps
  description: Install-only tool; its update re-runs install in place
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: echo "${REF}" >> "${WORKDIR}/install-refs"` + failCmd + `
          title: Install
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`)
}

// recipeManifest names its recipe in every install line, and its update can
// be made to fail, so a test can tell which release's steps ran for which
// ${REF}.
func recipeManifest(recipe string, updateFails bool) []byte {
	update := `echo "${REF}" >> "${WORKDIR}/update-refs"`
	if updateFails {
		update = "exit 7"
	}
	return []byte(fmt.Sprintf(`schema: "arrow@v0"
metadata:
  name: quiver-test.recipe-tool
  description: Records which release's recipe installed which ref
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: echo "%s ${REF}" >> "${WORKDIR}/install-log"
          title: Install
          timeout: 10s
          exit_on_failure: true
      update:
        - type: run
          command: %s
          title: Update
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
`, recipe, update))
}

// A newer release published and recorded while an update runs toward an
// older one must still be offered once that update commits: the commit
// advances to the older target and must not erase what a check found ahead
// of it (spec §8.2 step 7: "never ... a missed one"; §7.1).
func (s *AdvanceSuite) TestAdversarial_NewerReleaseRecordedDuringUpdate_StaysOffered() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.2.0", manifest)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("stable")

	s.install(env, tc, ns)
	v130 := s.publish(f, "v1.3.0", manifest)

	s.touchWorkFile(env, ns, holdFile)
	s.update(tc, ns)
	env.WaitForState(s.T(), ns, domain.ArrowStateUpdating, wait)

	v140 := s.publish(f, "v1.4.0", manifest)
	during := s.checkAvailable(tc, ns)
	s.Require().NotNil(during)
	s.Require().Equal(dto.AvailableDTO{Ref: "v1.4.0", Commit: v140}, *during, "the check during the update sees v1.4.0")

	s.touchWorkFile(env, ns, releaseFile)
	s.runtimeSettled(env, ns)

	after := s.detail(tc, ns)
	s.Equal(v130, after.InstalledCommit, "the update committed the target it ran")
	s.Require().NotNil(after.Available, "v1.4.0 is still ahead of the row: it must stay offered")
	s.Equal("v1.4.0", after.Available.Ref)
	s.True(after.Outdated)
	s.Equal(string(domain.ArrowStateOutdated), after.State, "the runtime badge must show v1.4.0 is ahead")
}

// A branch pin written with the refs/heads/ escape (the documented way to
// follow a branch that shares its name with a tag) must be able to commit
// an update: the commit's re-check must read the branch, not the tag.
func (s *AdvanceSuite) TestAdversarial_EscapedBranchPin_SameNameTag_UpdateCommits() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.0.0", manifest)
	s.Repos.Mutate(func() {
		head, err := f.storer.Reference(plumbing.NewBranchReferenceName("master"))
		s.Require().NoError(err)
		s.Require().NoError(f.storer.SetReference(
			plumbing.NewHashReference(plumbing.NewTagReferenceName("master"), head.Hash()),
		))
	})
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("refs/heads/master")

	s.install(env, tc, ns)
	s.Require().Equal("pin", s.detail(tc, ns).SelectorKind)

	var moved string
	s.Repos.Mutate(func() {
		kit.AddCommitToRepo(s.T(), f.storer, release(manifest, "branch-moved"))
		ref, err := f.storer.Reference(plumbing.NewBranchReferenceName("master"))
		s.Require().NoError(err)
		moved = ref.Hash().String()
	})
	available := s.checkAvailable(tc, ns)
	s.Require().NotNil(available, "the branch moved")
	s.Require().Equal(moved, available.Commit)

	s.update(tc, ns)
	s.runtimeSettled(env, ns)
	s.Equal([]string{"master"}, s.updateRuns(env, ns), "the update steps ran")

	after := s.detail(tc, ns)
	s.Equal(moved, after.InstalledCommit, "the branch did not move again: the update must be stamped")
	s.Nil(after.Available)
}

// After an update fails, the row keeps the target's manifest staged while
// Resolved stays at the old release. Reinstalling must not run one
// release's recipe with another release's ${REF}.
func (s *AdvanceSuite) TestAdversarial_FailedUpdateThenReinstall_RecipeMatchesRef() {
	f := s.newFixture("recipe-tool", "v1.2.0", recipeManifest("recipe-v1.2.0", false))
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("stable")

	s.install(env, tc, ns)
	s.publish(f, "v1.3.0", recipeManifest("recipe-v1.3.0", true))

	s.update(tc, ns)
	kit.WaitForDetail(s.T(), tc, ns, "the update failed", wait, func(d dto.ArrowDetailDTO, status int) bool {
		return status == http.StatusOK && d.LastReturn != nil &&
			d.LastReturn.Method == domain.MethodUpdate && d.LastReturn.Outcome != "success"
	})
	s.runtimeSettled(env, ns)
	failed := s.detail(tc, ns)
	s.Equal("v1.2.0", failed.ResolvedRef, "a failed update stamps nothing")

	s.Require().Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, wait)
	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	// v1.3.0 is still ahead of the reinstalled row, so the runtime may pass
	// ready on its way to outdated: wait for the install's own return.
	kit.WaitForDetail(s.T(), tc, ns, "the reinstall ended", wait, func(d dto.ArrowDetailDTO, status int) bool {
		return status == http.StatusOK && d.LastReturn != nil &&
			d.LastReturn.Method == domain.MethodInstall && d.LastReturn.Outcome == "success"
	})
	s.runtimeSettled(env, ns)

	log, ok := s.workFile(env, ns, "install-log")
	s.Require().True(ok)
	lines := strings.Split(strings.TrimSpace(log), "\n")
	last := lines[len(lines)-1]
	reinstalled := s.detail(tc, ns)
	s.Equal("recipe-"+reinstalled.ResolvedRef+" "+reinstalled.ResolvedRef, last,
		"${REF} and the recipe must name the release the row reports installed; the failed update's "+
			"LastReturn ${REF} must not leak into the reinstall")
}

// PATCH advances a row nothing is installed from (spec §8.1); the install
// that follows must install the release the row now resolves to. The
// uninstall's LastReturn carries the old ${REF}, which must not leak into it.
func (s *AdvanceSuite) TestAdversarial_AdvancedUninstalledRow_InstallsItsResolvedRef() {
	f := s.newFixture("recipe-tool", "v1.2.0", recipeManifest("recipe-v1.2.0", false))
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("stable")

	s.install(env, tc, ns)
	s.Require().Equal(http.StatusAccepted, tc.Uninstall(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateAbsent, wait)

	v130 := s.publish(f, "v1.3.0", recipeManifest("recipe-v1.3.0", false))
	_, status := tc.CheckAvailable(ns)
	s.Require().Equal(http.StatusOK, status)
	advanced := s.detail(tc, ns)
	s.Require().Equal("v1.3.0", advanced.ResolvedRef, "PATCH advances a row nothing is installed from")
	s.Require().Equal(v130, advanced.InstalledCommit)

	s.Require().Equal(http.StatusAccepted, tc.Install(ns, nil))
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, wait)
	s.runtimeSettled(env, ns)

	log, ok := s.workFile(env, ns, "install-log")
	s.Require().True(ok)
	lines := strings.Split(strings.TrimSpace(log), "\n")
	s.Equal("recipe-v1.3.0 v1.3.0", lines[len(lines)-1],
		"the install runs v1.3.0's recipe for ${REF}=v1.3.0, the ref the row resolves to")
}

// An arrow with no update: steps re-runs its install steps in place: the
// workdir survives, ${REF} is the target, the row advances.
func (s *AdvanceSuite) TestAdversarial_NoUpdateSteps_ReRunsInstallInPlace() {
	f := s.newFixture("no-update-steps", "v1.2.0", noUpdateStepsManifest(false))
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("stable")

	s.install(env, tc, ns)
	s.touchWorkFile(env, ns, "user-config")
	v130 := s.publish(f, "v1.3.0", noUpdateStepsManifest(false))

	s.update(tc, ns)
	s.waitAdvanced(tc, ns, v130)

	refs, ok := s.workFile(env, ns, "install-refs")
	s.Require().True(ok)
	s.Equal([]string{"v1.2.0", "v1.3.0"}, strings.Fields(refs), "${REF} is the target ref during the in-place re-install")
	_, kept := s.workFile(env, ns, "user-config")
	s.True(kept, "an update never wipes the workdir")

	s.Equal(http.StatusOK, tc.Execute(ns, domain.MethodUpdate, nil), "a second update is a no-op")
	refs, _ = s.workFile(env, ns, "install-refs")
	s.Equal([]string{"v1.2.0", "v1.3.0"}, strings.Fields(refs))
}

// A target whose (re-run) install fails leaves the old install usable and
// the row where it was.
func (s *AdvanceSuite) TestAdversarial_NoUpdateSteps_FailingTargetLeavesOldInstall() {
	f := s.newFixture("no-update-steps", "v1.2.0", noUpdateStepsManifest(false))
	installed := s.tagCommit(f, "v1.2.0")
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("stable")

	s.install(env, tc, ns)
	v130 := s.publish(f, "v1.3.0", noUpdateStepsManifest(true))

	s.update(tc, ns)
	kit.WaitForDetail(s.T(), tc, ns, "the update failed", wait, func(d dto.ArrowDetailDTO, status int) bool {
		return status == http.StatusOK && d.LastReturn != nil &&
			d.LastReturn.Method == domain.MethodUpdate && d.LastReturn.Outcome != "success"
	})
	s.runtimeSettled(env, ns)

	after := s.detail(tc, ns)
	s.Equal(installed, after.InstalledCommit)
	s.Require().NotNil(after.Available)
	s.Equal(v130, after.Available.Commit)
	s.Contains([]string{string(domain.ArrowStateReady), string(domain.ArrowStateOutdated)}, after.State,
		"a failed update leaves the row installed")
}

// A rolling tag moved back to an older commit is still a move: the row
// follows the tag, whichever way it went.
func (s *AdvanceSuite) TestAdversarial_RollingTagMovedBackwards_Followed() {
	f := s.rollingFixture()
	original := s.tagCommit(f, "nightly")
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("nightly")

	s.install(env, tc, ns)
	moved := s.moveTag(f, "nightly")
	s.update(tc, ns)
	s.waitAdvanced(tc, ns, moved)

	s.Repos.Mutate(func() {
		s.Require().NoError(f.storer.SetReference(plumbing.NewHashReference(
			plumbing.NewTagReferenceName("nightly"), plumbing.NewHash(original),
		)))
	})
	available := s.checkAvailable(tc, ns)
	s.Require().NotNil(available)
	s.Equal(original, available.Commit)
	s.update(tc, ns)
	s.waitAdvanced(tc, ns, original)
	s.Equal([]string{"nightly", "nightly"}, s.updateRuns(env, ns))
}

// A rolling tag deleted then recreated: while it is gone the row stays
// readable and nothing is recorded; once it is back at a new commit, the row
// updates to it.
func (s *AdvanceSuite) TestAdversarial_RollingTagDeletedThenRecreated() {
	f := s.rollingFixture()
	installed := s.tagCommit(f, "nightly")
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("nightly")

	s.install(env, tc, ns)
	var tagged plumbing.Hash
	s.Repos.Mutate(func() {
		ref, err := f.storer.Reference(plumbing.NewTagReferenceName("nightly"))
		s.Require().NoError(err)
		tagged = ref.Hash()
		s.Require().NoError(f.storer.RemoveReference(plumbing.NewTagReferenceName("nightly")))
	})

	_, patchStatus := tc.CheckAvailable(ns)
	s.NotEqual(http.StatusOK, patchStatus, "a vanished rolling tag has no answer")
	s.Less(patchStatus, 500, "a vanished tag is a client-visible condition, not a server error")
	updateStatus := tc.Execute(ns, domain.MethodUpdate, nil)
	s.Less(updateStatus, 500, "update of a row whose tag vanished must not be a 5xx")
	gone := s.detail(tc, ns)
	s.Equal(installed, gone.InstalledCommit)
	s.Nil(gone.Available, "nothing is recorded while the tag is missing")

	s.Repos.Mutate(func() {
		s.Require().NoError(f.storer.SetReference(plumbing.NewHashReference(plumbing.NewTagReferenceName("nightly"), tagged)))
	})
	moved := s.moveTag(f, "nightly")
	s.update(tc, ns)
	s.waitAdvanced(tc, ns, moved)
}

// Two selectors of one repository updated at the same moment both advance,
// each in its own workdir.
func (s *AdvanceSuite) TestAdversarial_TwoSelectorsUpdatedAtOnce() {
	manifest := s.stableManifest()
	f := s.newFixture("stable-tool", "v1.2.0", manifest)
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	channelNS, constraintNS := f.ns("stable"), f.ns("v1.*")

	s.install(env, tc, channelNS)
	s.install(env, tc, constraintNS)
	v130 := s.publish(f, "v1.3.0", manifest)

	var wg sync.WaitGroup
	statuses := make([]int, 2)
	for i, ns := range []string{channelNS, constraintNS} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i] = tc.Execute(ns, domain.MethodUpdate, nil)
		}()
	}
	wg.Wait()
	s.Equal([]int{http.StatusAccepted, http.StatusAccepted}, statuses)
	s.waitAdvanced(tc, channelNS, v130)
	s.waitAdvanced(tc, constraintNS, v130)
	s.Equal([]string{"v1.3.0"}, s.updateRuns(env, channelNS))
	s.Equal([]string{"v1.3.0"}, s.updateRuns(env, constraintNS))
}

// Concurrent updates and checks of one row: exactly one update runs, the
// others are refused or no-ops, and the row ends current.
func (s *AdvanceSuite) TestAdversarial_ConcurrentUpdatesAndChecks_OneRuns() {
	f := s.rollingFixture()
	env := s.NewEnv()
	tc := env.TypedClient(s.T())
	ns := f.ns("nightly")

	s.install(env, tc, ns)
	moved := s.moveTag(f, "nightly")

	var wg sync.WaitGroup
	var mu sync.Mutex
	statuses := map[int]int{}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var status int
			if i%2 == 0 {
				status = tc.Execute(ns, domain.MethodUpdate, nil)
			} else {
				_, status = tc.CheckAvailable(ns)
			}
			mu.Lock()
			statuses[status]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	s.Equal(1, statuses[http.StatusAccepted], "exactly one update begins: %v", statuses)
	for status := range statuses {
		s.Less(status, 500, "no concurrent call may fail with a server error: %v", statuses)
	}
	s.waitAdvanced(tc, ns, moved)
	s.Equal([]string{"nightly"}, s.updateRuns(env, ns))
}

// A second update requested after the first one's steps ended but before its
// detached commit landed (the row still reports settling) must not run the
// update steps again for the target the first already installed.
func (s *AdvanceSuite) TestAdversarial_UpdateDuringCommitWindow_DoesNotRerunSteps() {
	f := s.rollingFixture()
	gate := newCommitGate()
	env := s.NewEnv(kit.WithManifoldWrapper(gate.wrap))
	tc := env.TypedClient(s.T())
	ns := f.ns("nightly")
	moved := s.updateHeldInCommit(env, gate, f)
	env.WaitForState(s.T(), ns, domain.ArrowStateReady, wait)

	second := tc.Execute(ns, domain.MethodUpdate, nil)
	s.NotEqual(http.StatusAccepted, second,
		"the first update of %s is still settling: a second bracket must not begin toward the same target", moved)

	gate.release <- nil
	s.waitAdvanced(tc, ns, moved)
	s.runtimeSettled(env, ns)
	s.Equal([]string{"nightly"}, s.updateRuns(env, ns), "the update steps ran once for one target")
}
