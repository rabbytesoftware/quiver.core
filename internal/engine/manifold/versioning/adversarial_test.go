package versioning

import (
	"fmt"
	"math/rand"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	resolvers "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
	sel "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/versioning/internal/selector"
)

var adversarialTagVocab = []string{
	"v1.0.0", "v1.2.0", "v1.10.0", "v1.9.0", "v2.0.0", "1.2.0", "v1.2", "v1.2.0-rc.1", "v2.0.0-beta.1",
	"v2.0.0-beta.2", "nightly", "nightly-latest", "v1.0-latest", "stable", "beta", "latest", "main",
	"release-1.0", "release-1.1", "beta-2.0", "abcdef1", "deadbeefcafe", "a/b", "feature/x", "v1.*",
	"Nightly", "v3.0.0-Beta", "x@y", "ref:colon", "q?mark", "nightly-2026.09.30", "insiders",
}

var adversarialBranchVocab = []string{"main", "develop", "nightly", "feature/x", "release/1.0", "stable", "abcdef1"}

func adversarialCommit(r *rand.Rand) string {
	const hex = "0123456789abcdef"
	b := make([]byte, 40)
	for i := range b {
		b[i] = hex[r.Intn(len(hex))]
	}
	return string(b)
}

func adversarialSnapshot(r *rand.Rand) domain.RefSnapshot {
	snap := domain.RefSnapshot{Tags: map[string]string{}, Branches: map[string]string{}}
	for _, tag := range adversarialTagVocab {
		if r.Intn(3) == 0 {
			snap.Tags[tag] = adversarialCommit(r)
		}
	}
	for _, branch := range adversarialBranchVocab {
		if r.Intn(3) == 0 {
			snap.Branches[branch] = adversarialCommit(r)
		}
	}
	if len(snap.Branches) > 0 && r.Intn(4) != 0 {
		names := make([]string, 0, len(snap.Branches))
		for b := range snap.Branches {
			names = append(names, b)
		}
		sort.Strings(names)
		snap.Head = names[r.Intn(len(names))]
	}
	return snap
}

func adversarialSelectors(snap domain.RefSnapshot) []string {
	selectors := []string{
		"stable", "beta", "rc", "nightly", "latest", "v1.*", "v2.*", "v*", "*", "v1.[0-9]*", "[", "v1.2.?",
		"refs/tags/stable", "refs/heads/main", "refs/tags/nightly", "ABCDEF1", "abcdef1234", "/x", "x/", "a//b",
		"", "@", ":", "%2A", strings.Repeat("a", 300),
	}
	for tag := range snap.Tags {
		selectors = append(selectors, tag, "refs/tags/"+tag)
	}
	for branch := range snap.Branches {
		selectors = append(selectors, branch, "refs/heads/"+branch)
	}
	for _, c := range ChannelsOf(snap) {
		selectors = append(selectors, c.Name)
	}
	return selectors
}

// TestAdversarial_Drift_PropertyInvariants checks the model's invariants over
// random snapshots: nothing panics, a pin never leaves its own ref, a
// constraint never leaves its glob, a commit is never outdated, and a row
// advanced onto its target is current against the same snapshot.
func TestAdversarial_Drift_PropertyInvariants(t *testing.T) {
	r := rand.New(rand.NewSource(20260930)) // #nosec G404 -- deterministic fuzz seed
	for iteration := 0; iteration < 3000; iteration++ {
		snap := adversarialSnapshot(r)
		for _, selector := range adversarialSelectors(snap) {
			kind, err := ClassifySelector(selector, snap)
			if err != nil {
				continue
			}
			if _, targetErr := Target(kind, selector, snap); targetErr != nil && kind == domain.SelectorConstraint {
				continue
			}
			target, outdated, err := Drift(kind, selector, domain.Resolved{}, snap)
			if kind == domain.SelectorCommit {
				assert.False(t, outdated, "commit %q is never outdated", selector)
				continue
			}
			require.NoError(t, err, "a classified selector %q (%s) must resolve against the snapshot it was classified on: %+v", selector, kind, snap)
			require.True(t, outdated, "an empty Resolved is outdated")

			switch kind.Family() {
			case domain.SelectorPin:
				want := strings.TrimPrefix(strings.TrimPrefix(selector, "refs/tags/"), "refs/heads/")
				assert.Equal(t, want, target.Ref, "pin %q never crosses its ref", selector)
			case domain.SelectorConstraint:
				matched, _ := path.Match(selector, target.Ref)
				assert.True(t, matched, "constraint %q never crosses its bounds (got %q)", selector, target.Ref)
				_, isTag := snap.Tags[target.Ref]
				assert.True(t, isTag, "constraint %q resolves to a tag", selector)
			case domain.SelectorChannel, domain.SelectorCommit, domain.SelectorTagPin, domain.SelectorBranchPin,
				domain.SelectorOrderedChannel, domain.SelectorPointerChannel, domain.SelectorBranchChannel:
			}

			_, again, err := Drift(kind, selector, domain.Resolved{Ref: target.Ref, Commit: target.Commit}, snap)
			require.NoError(t, err)
			assert.False(t, again, "Drift is idempotent once Resolved is advanced to its target: %q (%s)", selector, kind)
		}
	}
}

// TestAdversarial_Target_UnrelatedTagNeverMovesTarget adds a tag no selector
// could mean and requires every classified row's target to stay put: a later
// tag push never changes what an existing identity points at (spec §2.3).
func TestAdversarial_Target_UnrelatedTagNeverMovesTarget(t *testing.T) {
	r := rand.New(rand.NewSource(7)) // #nosec G404 -- deterministic fuzz seed
	var failures []string
	for iteration := 0; iteration < 2000 && len(failures) < 5; iteration++ {
		snap := adversarialSnapshot(r)
		later := domain.RefSnapshot{Tags: map[string]string{}, Branches: snap.Branches, Head: snap.Head}
		for k, v := range snap.Tags {
			later.Tags[k] = v
		}
		later.Tags["0-unrelated"] = adversarialCommit(r)

		for _, selector := range adversarialSelectors(snap) {
			kind, err := ClassifySelector(selector, snap)
			if err != nil || kind == domain.SelectorCommit {
				continue
			}
			before, err := Target(kind, selector, snap)
			if err != nil {
				continue
			}
			after, err := Target(kind, selector, later)
			if err != nil || before != after {
				failures = append(failures, fmt.Sprintf("%q (%s): %+v -> %+v err=%v tags=%v", selector, kind, before, after, err, sel.SortedTags(snap)))
			}
		}
	}
	assert.Empty(t, failures, "an unrelated tag must never move a selector's target")
}

// TestAdversarial_PointerChannel_HijackedByLaterOrderedChannel: a row
// following the rolling tag "nightly" (a pointer channel) switches to
// following dated "nightly-*" builds as soon as the first one is published,
// because findChannel prefers an ordered channel of the same name.
func TestAdversarial_PointerChannel_HijackedByLaterOrderedChannel(t *testing.T) {
	before := domain.RefSnapshot{Tags: map[string]string{"v1.2.0": "c120", "nightly": "rolling1"}}
	kind, err := ClassifySelector("nightly", before)
	require.NoError(t, err)
	require.Equal(t, domain.SelectorChannel, kind.Family())
	target, err := Target(kind, "nightly", before)
	require.NoError(t, err)
	require.Equal(t, domain.Available{Ref: "nightly", Commit: "rolling1"}, target)

	later := domain.RefSnapshot{Tags: map[string]string{
		"v1.2.0": "c120", "nightly": "rolling2", "nightly-2026.09.30": "dated",
	}}
	_, outdated, err := Drift(kind, "nightly", domain.Resolved{Ref: "nightly", Commit: "rolling1"}, later)
	require.NoError(t, err)
	target, _ = Target(kind, "nightly", later)
	assert.True(t, outdated)
	assert.Equal(t, "nightly", target.Ref,
		"crowbar@nightly follows the rolling tag it was installed from; a dated build tag must not re-point the identity")
}

// TestAdversarial_BranchPin_HijackedByLaterSameNameTag: a row pinned to the
// branch "develop" silently follows a tag named "develop" once one is pushed.
func TestAdversarial_BranchPin_HijackedByLaterSameNameTag(t *testing.T) {
	before := domain.RefSnapshot{Tags: map[string]string{"v1.0.0": "c1"}, Branches: map[string]string{"develop": "b1"}}
	kind, err := ClassifySelector("develop", before)
	require.NoError(t, err)
	require.Equal(t, domain.SelectorPin, kind.Family())

	later := domain.RefSnapshot{
		Tags:     map[string]string{"v1.0.0": "c1", "develop": "old-tag"},
		Branches: map[string]string{"develop": "b1"},
	}
	_, outdated, err := Drift(kind, "develop", domain.Resolved{Ref: "develop", Commit: "b1"}, later)
	require.NoError(t, err)
	assert.False(t, outdated, "the develop branch did not move; a same-name tag must not re-point a branch pin")
}

// TestAdversarial_StableChannel_VanishesWhenPrefixesBecomeMeaningful: a
// repository that tags release-1.0, release-1.1 has them in "stable"; a
// later beta-2.0 makes every prefix meaningful, "stable" disappears, and the
// row can never be checked or updated again.
func TestAdversarial_StableChannel_VanishesWhenPrefixesBecomeMeaningful(t *testing.T) {
	before := domain.RefSnapshot{Tags: map[string]string{"release-1.0": "c10", "release-1.1": "c11"}}
	selector, err := DefaultChannel(before)
	require.NoError(t, err)
	require.Equal(t, "stable", selector)
	kind, err := ClassifySelector(selector, before)
	require.NoError(t, err)
	target, err := Target(kind, selector, before)
	require.NoError(t, err)
	require.Equal(t, "release-1.1", target.Ref)

	later := domain.RefSnapshot{Tags: map[string]string{"release-1.0": "c10", "release-1.1": "c11", "beta-2.0": "c20"}}
	_, _, err = Drift(kind, selector, domain.Resolved{Ref: "release-1.1", Commit: "c11"}, later)
	assert.NoError(t, err, "a published beta must not make crowbar@stable unresolvable")
}

// TestAdversarial_Constraint_EquivalentTagsTieIsDeterministic: two tags that
// rank equal (v1.2 and v1.2.0 at different commits) must resolve to the same
// one whatever else the repository holds, or the row flaps between them.
func TestAdversarial_Constraint_EquivalentTagsTieIsDeterministic(t *testing.T) {
	seen := map[string]bool{}
	for extra := 0; extra < 40; extra++ {
		snap := domain.RefSnapshot{Tags: map[string]string{"v1.2": "short", "v1.2.0": "long"}}
		for i := 0; i < extra; i++ {
			snap.Tags[fmt.Sprintf("v1.1.%d", i)] = fmt.Sprintf("old%d", i)
		}
		target, err := Target(domain.SelectorConstraint, "v1.*", snap)
		require.NoError(t, err)
		seen[target.Ref] = true
		stable, err := Target(domain.SelectorChannel, "stable", snap)
		require.NoError(t, err)
		seen["stable:"+stable.Ref] = true
	}
	constraintAnswers, channelAnswers := 0, 0
	for k := range seen {
		if strings.HasPrefix(k, "stable:") {
			channelAnswers++
			continue
		}
		constraintAnswers++
	}
	assert.Equal(t, 1, constraintAnswers, "v1.* must settle on one of two equal tags, got %v", seen)
	assert.Equal(t, 1, channelAnswers, "stable must settle on one of two equal tags, got %v", seen)
}

// TestAdversarial_Admit_DefaultBranchFallbackAfterFirstTag: a row that
// settled on the default branch keeps following it after the first release
// (Drift honours that); adopting the branch's own ref must be admitted too.
func TestAdversarial_Admit_DefaultBranchFallbackAfterFirstTag(t *testing.T) {
	branchOnly := domain.RefSnapshot{Branches: map[string]string{"main": "m1"}, Head: "main"}
	kind, err := ClassifySelector("main", branchOnly)
	require.NoError(t, err)
	require.Equal(t, domain.SelectorChannel, kind.Family())

	released := domain.RefSnapshot{
		Tags: map[string]string{"v1.0.0": "c1"}, Branches: map[string]string{"main": "m2"}, Head: "main",
	}
	target, outdated, err := Drift(kind, "main", domain.Resolved{Ref: "main", Commit: "m1"}, released)
	require.NoError(t, err)
	require.True(t, outdated)
	require.Equal(t, "main", target.Ref)

	admitted, err := Admit(kind, "main", "main", released)
	assert.NoError(t, err, "the ref Drift itself offers the row must be admitted by /adopt")
	assert.Equal(t, "m2", admitted.Commit)
}

// TestAdversarial_ClassifySelector_ManyChannelsStaysFast measures a check
// against a repository with thousands of release tags spread over many
// suffix channels: per-build suffixes such as v0.1.0-nightly.20260930.abc1234
// do not fit the letters[-_.]digits shape, so each is its own channel.
func TestAdversarial_ClassifySelector_ManyChannelsStaysFast(t *testing.T) {
	snap := domain.RefSnapshot{Tags: map[string]string{}}
	for i := 0; i < 3000; i++ {
		snap.Tags[fmt.Sprintf("v0.1.0-nightly.2026%04d.%07x", i, i*7919)] = fmt.Sprintf("c%d", i)
	}
	snap.Tags["nightly"] = "n"
	start := time.Now()
	kind, err := ClassifySelector("nightly", snap)
	require.NoError(t, err)
	_, _, err = Drift(kind, "nightly", domain.Resolved{}, snap)
	require.NoError(t, err)
	elapsed := time.Since(start)
	t.Logf("classify+drift over %d tags / %d channels: %s", len(snap.Tags), len(resolvers.ChannelsPresent(sel.SortedTags(snap))), elapsed)
	assert.Less(t, elapsed, 5*time.Second, "a version check must not take seconds of CPU on a busy repository")
}
