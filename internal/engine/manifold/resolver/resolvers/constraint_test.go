package resolvers

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

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

// ─── Resolve (public) ─────────────────────────────────────────────────────────

// ─── ListTags (public) ────────────────────────────────────────────────────────

// ─── DefaultBranch ────────────────────────────────────────────────────────────

// makeRepoOnBranch builds a repo whose default branch is exactly branch, so a
// name outside the usual main/master pair can be exercised.
func makeRepoOnBranch(
	t *testing.T,
	branch string,
) string {
	t.Helper()

	dir := t.TempDir()

	repo, err := gogit.PlainInitWithOptions(dir, &gogit.PlainInitOptions{
		InitOptions: gogit.InitOptions{
			DefaultBranch: plumbing.NewBranchReferenceName(branch),
		},
	})
	if err != nil {
		t.Fatalf("PlainInitWithOptions: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	if err := os.WriteFile(dir+"/arrow.yaml", []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := wt.Add("arrow.yaml"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := wt.Commit("init", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com"},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	return dir
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

// ─── RefCommit ────────────────────────────────────────────────────────────────

func commitOnRepo(
	t *testing.T,
	dir string,
	name string,
) plumbing.Hash {
	t.Helper()

	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(dir+"/"+name, []byte(name), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := wt.Add(name); err != nil {
		t.Fatalf("Add: %v", err)
	}
	hash, err := wt.Commit(name, &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "test@test.com"},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return hash
}

// ─── Refs ─────────────────────────────────────────────────────────────────────

func TestConstraintResolver_Refs_SnapshotsTagsBranchesAndHead(t *testing.T) {
	dir := makeRepoOnBranch(t, "develop")
	repo, err := gogit.PlainOpen(dir)
	require.NoError(t, err)
	head, err := repo.Head()
	require.NoError(t, err)
	first := head.Hash()

	_, err = repo.CreateTag("v1.0.0", first, nil)
	require.NoError(t, err)
	_, err = repo.CreateTag("v1.1.0", first, &gogit.CreateTagOptions{
		Tagger:  &object.Signature{Name: "test", Email: "test@test.com"},
		Message: "annotated",
	})
	require.NoError(t, err)
	nightly := plumbing.NewTagReferenceName("nightly")
	require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(nightly, first)))

	second := commitOnRepo(t, dir, "second")
	require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(nightly, second)))
	require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("feature"), first)))

	cr := newCR(5 * time.Second)
	snap, err := cr.refsWithCloneURL(context.Background(), dir)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{
		"v1.0.0":  first.String(),
		"v1.1.0":  first.String(),
		"nightly": second.String(),
	}, snap.Tags)
	assert.Equal(t, map[string]string{
		"develop": second.String(),
		"feature": first.String(),
	}, snap.Branches)
	assert.Equal(t, "develop", snap.Head)
}

func TestConstraintResolver_Refs_EmptyRepoHasNoRefsAndNoError(t *testing.T) {
	dir := t.TempDir()
	_, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	cr := newCR(5 * time.Second)
	snap, err := cr.refsWithCloneURL(context.Background(), dir)
	require.NoError(t, err)

	assert.Empty(t, snap.Tags)
	assert.Empty(t, snap.Branches)
	assert.Equal(t, "", snap.Head)
}

func TestConstraintResolver_Refs_UnreachableRemote(t *testing.T) {
	cr := newCR(500 * time.Millisecond)

	_, err := cr.refsWithCloneURL(context.Background(), "/nonexistent/path/to/nowhere")
	assert.Error(t, err)
}

func TestConstraintResolver_Refs_ReturnsErrorForUnresolvableNS(t *testing.T) {
	cr := NewConstraintResolver(500 * time.Millisecond)

	_, err := cr.Refs(context.Background(), domain.Namespace("localhost/user/nonexistent"))
	assert.Error(t, err)
}

func TestSnapshotOf_Advertisement(t *testing.T) {
	tagObject := plumbing.NewHash("1111111111111111111111111111111111111111")
	commit := plumbing.NewHash("2222222222222222222222222222222222222222")
	branch := plumbing.NewHash("3333333333333333333333333333333333333333")

	testCases := []struct {
		name string
		refs []*plumbing.Reference
		want domain.RefSnapshot
	}{
		{
			name: "peeled entry overrides its annotated tag whatever the order",
			refs: []*plumbing.Reference{
				plumbing.NewHashReference(plumbing.NewTagReferenceName("v1.0.0^{}"), commit),
				plumbing.NewHashReference(plumbing.NewTagReferenceName("v1.0.0"), tagObject),
			},
			want: domain.RefSnapshot{
				Tags:     map[string]string{"v1.0.0": commit.String()},
				Branches: map[string]string{},
			},
		},
		{
			name: "HEAD pointing at an unadvertised branch leaves Head empty",
			refs: []*plumbing.Reference{
				plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("develop")),
				plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), branch),
			},
			want: domain.RefSnapshot{
				Tags:     map[string]string{},
				Branches: map[string]string{"main": branch.String()},
			},
		},
		{
			name: "HEAD pointing outside refs/heads leaves Head empty",
			refs: []*plumbing.Reference{
				plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewTagReferenceName("v1.0.0")),
				plumbing.NewHashReference(plumbing.NewTagReferenceName("v1.0.0"), commit),
			},
			want: domain.RefSnapshot{
				Tags:     map[string]string{"v1.0.0": commit.String()},
				Branches: map[string]string{},
			},
		},
		{
			name: "refs that are neither tag nor branch are ignored",
			refs: []*plumbing.Reference{
				plumbing.NewHashReference(plumbing.ReferenceName("refs/pull/1/head"), commit),
			},
			want: domain.RefSnapshot{
				Tags:     map[string]string{},
				Branches: map[string]string{},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, snapshotOf(tc.refs))
		})
	}
}

// ─── HighestMatch ─────────────────────────────────────────────────────────────

func TestHighestMatch(t *testing.T) {
	testCases := []struct {
		name    string
		tags    []string
		pattern string
		want    string
		wantOK  bool
		wantErr error
	}{
		{name: "highest semver within the glob", tags: []string{"v1.2.0", "v1.10.0", "v2.0.0"}, pattern: "v1.*", want: "v1.10.0", wantOK: true},
		{name: "no tag matches", tags: []string{"v2.0.0"}, pattern: "v1.*"},
		{name: "no tags at all", tags: nil, pattern: "*"},
		{name: "bad pattern", tags: []string{"v1.0.0"}, pattern: "v1.[", wantErr: path.ErrBadPattern},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := HighestMatch(tc.tags, tc.pattern)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestHighestMatch_DoesNotReorderTheCallersSlice(t *testing.T) {
	tags := []string{"v1.0.0", "v1.2.0"}
	_, _, err := HighestMatch(tags, "*")
	require.NoError(t, err)
	assert.Equal(t, []string{"v1.0.0", "v1.2.0"}, tags)
}
