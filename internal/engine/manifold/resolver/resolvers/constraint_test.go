package resolvers

import (
	"context"
	"os"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/rabbytesoftware/quiver/internal/domain"
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

// ─── isSemver ─────────────────────────────────────────────────────────────────

func TestIsSemver_Valid(t *testing.T) {
	cases := []string{"v1.0.0", "v2.3.4", "v1.2"}
	for _, tc := range cases {
		if !isSemver(tc) {
			t.Errorf("isSemver(%q) = false, want true", tc)
		}
	}
}

func TestIsSemver_Invalid(t *testing.T) {
	cases := []string{"nightly", "beta-1", ""}
	for _, tc := range cases {
		if isSemver(tc) {
			t.Errorf("isSemver(%q) = true, want false", tc)
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

func TestIsSemver_NegativeComponent(t *testing.T) {
	// strconv.Atoi parses "-1" as -1 (no error), the n < 0 guard must catch it.
	if isSemver("v1.-1.0") {
		t.Error("isSemver(v1.-1.0) = true, want false (negative component)")
	}
}
