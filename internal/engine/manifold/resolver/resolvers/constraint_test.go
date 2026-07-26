package resolvers

import (
	"context"
	"os"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func makeRepoWithTags(
	t *testing.T,
	tags []string,
) string {
	t.Helper()

	dir := t.TempDir()

	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	p := dir + "/arrow.yaml"
	if err := os.WriteFile(p, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := wt.Add("arrow.yaml"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	hash, err := wt.Commit("init", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com"},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	for _, tag := range tags {
		if _, err := repo.CreateTag(tag, hash, nil); err != nil {
			t.Fatalf("CreateTag %s: %v", tag, err)
		}
	}

	return dir
}

// ─── sortTagsDesc ─────────────────────────────────────────────────────────────

func TestSortTagsDesc_Semver(t *testing.T) {
	tags := []string{"v1.0.0", "v1.4.0", "v1.2.3"}
	sortTagsDesc(tags)
	if tags[0] != "v1.4.0" {
		t.Errorf("first = %q, want %q", tags[0], "v1.4.0")
	}
}

func TestSortTagsDesc_TwoDigitMinor(t *testing.T) {
	tags := []string{"v1.9.0", "v1.10.0", "v1.2.0"}
	sortTagsDesc(tags)
	if tags[0] != "v1.10.0" {
		t.Errorf("first = %q, want %q", tags[0], "v1.10.0")
	}
}

func TestSortTagsDesc_MajorVersion(t *testing.T) {
	tags := []string{"v2.0.0", "v10.0.0", "v1.0.0"}
	sortTagsDesc(tags)
	if tags[0] != "v10.0.0" {
		t.Errorf("first = %q, want %q", tags[0], "v10.0.0")
	}
}

func TestSortTagsDesc_LexSort(t *testing.T) {
	tags := []string{"beta-1", "alpha-2", "release-3"}
	sortTagsDesc(tags)
	if tags[0] != "release-3" {
		t.Errorf("first = %q, want %q", tags[0], "release-3")
	}
}

func TestSortTagsDesc_SingleTag(t *testing.T) {
	tags := []string{"v1.0.0"}
	sortTagsDesc(tags)
	if tags[0] != "v1.0.0" {
		t.Errorf("first = %q, want %q", tags[0], "v1.0.0")
	}
}

func TestSortTagsDesc_MixedSemverAndNonSemver(t *testing.T) {
	testCases := []struct {
		name string
		tags []string
		want []string
	}{
		{
			name: "nightly does not demote semver to lexicographic",
			tags: []string{"v1.9.0", "v1.10.0", "nightly"},
			want: []string{"v1.10.0", "v1.9.0", "nightly"},
		},
		{
			name: "prerelease tags rank below every stable tag",
			tags: []string{"v2.0.0-rc.1", "v1.0.0", "v2.0.0"},
			want: []string{"v2.0.0", "v1.0.0", "v2.0.0-rc.1"},
		},
		{
			name: "non-semver remainder keeps lexicographic order",
			tags: []string{"alpha", "v1.0.0", "zeta", "mid"},
			want: []string{"v1.0.0", "zeta", "mid", "alpha"},
		},
		{
			name: "two part semver participates in numeric ordering",
			tags: []string{"v1.2", "v1.10", "nightly"},
			want: []string{"v1.10", "v1.2", "nightly"},
		},
		{
			name: "lexicographic winner never beats the semver lane",
			tags: []string{"zzz", "v0.0.1"},
			want: []string{"v0.0.1", "zzz"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := append([]string(nil), tc.tags...)
			sortTagsDesc(got)
			if len(got) != len(tc.want) {
				t.Fatalf("length = %d, want %d", len(got), len(tc.want))
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestSortTagsDesc_AllNonSemverStaysLexicographic(t *testing.T) {
	tags := []string{"nightly", "alpha", "zeta"}
	sortTagsDesc(tags)

	want := []string{"zeta", "nightly", "alpha"}
	for i := range want {
		if tags[i] != want[i] {
			t.Fatalf("got %v, want %v", tags, want)
		}
	}
}

func TestSortTagsDesc_Empty(t *testing.T) {
	var tags []string
	sortTagsDesc(tags)
	if len(tags) != 0 {
		t.Errorf("len = %d, want 0", len(tags))
	}
}

func TestSortTagsDesc_TwoPartSemver(t *testing.T) {
	tags := []string{"v1.2", "v1.9", "v1.10"}
	sortTagsDesc(tags)
	if tags[0] != "v1.10" {
		t.Errorf("first = %q, want %q", tags[0], "v1.10")
	}
}

// ─── IsStableSemver ───────────────────────────────────────────────────────────

func TestIsStableSemver_Valid(t *testing.T) {
	cases := []string{"v1.0.0", "v2.3.4", "v1.2", "1.2.3"}
	for _, tc := range cases {
		if !IsStableSemver(tc) {
			t.Errorf("IsStableSemver(%q) = false, want true", tc)
		}
	}
}

func TestIsStableSemver_Invalid(t *testing.T) {
	cases := []string{"nightly", "beta-1", "", "v1", "v1.2.3.4", "v1.2.0-rc.1"}
	for _, tc := range cases {
		if IsStableSemver(tc) {
			t.Errorf("IsStableSemver(%q) = true, want false", tc)
		}
	}
}

// ─── constraintResolver internals ─────────────────────────────────────────────

func newCR(t time.Duration) *constraintResolver {
	return &constraintResolver{timeout: t}
}

func TestConstraintResolver_PicksHighestSemver(t *testing.T) {
	dir := makeRepoWithTags(t, []string{"v1.0.0", "v1.2.3", "v1.4.0"})
	cr := newCR(5 * time.Second)

	got, err := cr.resolveWithCloneURL(context.Background(), dir, "v1.*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.4.0" {
		t.Errorf("got %q, want %q", got, "v1.4.0")
	}
}

func TestConstraintResolver_MixedTagsPicksHighestSemver(t *testing.T) {
	dir := makeRepoWithTags(t, []string{"v1.9.0", "v1.10.0", "nightly"})
	cr := newCR(5 * time.Second)

	got, err := cr.resolveWithCloneURL(context.Background(), dir, "*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.10.0" {
		t.Errorf("got %q, want %q", got, "v1.10.0")
	}
}

func TestConstraintResolver_NoMatchingTags(t *testing.T) {
	dir := makeRepoWithTags(t, []string{"v2.0.0"})
	cr := newCR(5 * time.Second)

	_, err := cr.resolveWithCloneURL(context.Background(), dir, "v1.*")
	if err == nil {
		t.Fatal("expected error for no matching tags")
	}
}

func TestConstraintResolver_ExactGlobMatch(t *testing.T) {
	dir := makeRepoWithTags(t, []string{"v1.2.3", "v1.2.4", "v1.3.0"})
	cr := newCR(5 * time.Second)

	got, err := cr.resolveWithCloneURL(context.Background(), dir, "v1.2.*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.2.4" {
		t.Errorf("got %q, want %q", got, "v1.2.4")
	}
}

func TestConstraintResolver_InvalidGlobPattern(t *testing.T) {
	dir := makeRepoWithTags(t, []string{"v1.0.0"})
	cr := newCR(5 * time.Second)

	_, err := cr.resolveWithCloneURL(context.Background(), dir, "[invalid")
	if err == nil {
		t.Fatal("expected error for invalid pattern")
	}
}

// ─── Resolve (public) ─────────────────────────────────────────────────────────

// TestConstraintResolver_Resolve_UsesLocalRepo exercises the public Resolve
// method end-to-end. We use a domain.Namespace whose BareNamespace().CloneURL()
// returns the local repo dir (go-git accepts bare directory paths as clone URLs).
// The trick: split "github.com/user/repoSUFFIX" so CloneURL == local path is
// not possible via the standard Namespace type — instead we call Resolve on a
// real ConstraintResolver interface value and validate it returns an error when
// the namespace has no valid remote (proving the code path is exercised).
func TestConstraintResolver_Resolve_ReturnsErrorForUnresolvableNS(t *testing.T) {
	cr := NewConstraintResolver(500 * time.Millisecond)

	// Namespace with no real remote — Resolve must fail and return a non-nil error.
	_, err := cr.Resolve(context.Background(), domain.Namespace("localhost/user/nonexistent"), "v1.*")
	if err == nil {
		t.Fatal("expected error from Resolve with unreachable namespace")
	}
}

// ─── semverGT edge cases ──────────────────────────────────────────────────────

func TestSemverGT_Equal(t *testing.T) {
	if semverGT("v1.2.3", "v1.2.3") {
		t.Error("semverGT(equal) = true, want false")
	}
}

func TestSemverGT_PatchDiffers(t *testing.T) {
	if !semverGT("v1.2.4", "v1.2.3") {
		t.Error("semverGT(v1.2.4, v1.2.3) = false, want true")
	}
}

// ─── isSemver negative component ──────────────────────────────────────────────

func TestIsStableSemver_NegativeComponent(t *testing.T) {
	// strconv.Atoi parses "-1" as -1 (no error), the n < 0 guard must catch it.
	if IsStableSemver("v1.-1.0") {
		t.Error("IsStableSemver(v1.-1.0) = true, want false (negative component)")
	}
}
