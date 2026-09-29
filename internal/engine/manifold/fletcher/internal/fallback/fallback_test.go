package fallback_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/fallback"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/forge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/gather"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const fletchDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000001"

type stubDrafter struct {
	answer     func(tag string) (gather.Draft, error)
	fletchTags []string
}

func (s *stubDrafter) Draft(
	_ context.Context,
	_ domain.Namespace,
	tag string,
) (gather.Draft, error) {
	s.fletchTags = append(s.fletchTags, tag)
	return s.answer(tag)
}

type stubReleases struct {
	stable      string
	unstable    string
	unstableErr error
	branch      string
	branchErr   error
	stableCalls int
}

func (s *stubReleases) LatestStable(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	s.stableCalls++
	return s.stable, nil
}

func (s *stubReleases) LatestUnstable(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return s.unstable, s.unstableErr
}

func (s *stubReleases) ResolveDefaultBranch(
	_ context.Context,
	_ domain.Namespace,
) (string, string, error) {
	return s.branch, "", s.branchErr
}

type stubHost struct {
	branches []string
}

func (s *stubHost) RawFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", nil
}

func (s *stubHost) BlobFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", nil
}

func (s *stubHost) RepoPageURL(
	_ domain.Namespace,
) string {
	return ""
}

func (s *stubHost) DefaultBranches() []string { return s.branches }

func (s *stubHost) LatestRelease(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "", nil
}

func (s *stubHost) ReleaseAssets(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) ([]domain.ReleaseAsset, error) {
	return nil, nil
}

func hostedBy(
	host hosts.Host,
) hosts.Lookup {
	return func(_ domain.Namespace) (hosts.Host, bool) {
		return host, true
	}
}

func inferredManifest(
	t *testing.T,
) []byte {
	t.Helper()
	data, err := forge.Render(forge.Input{
		Repo:        "tool",
		Name:        "tool",
		Description: "A tool that does things.",
		URL:         "https://github.com/acme/tool",
		Generator:   domain.ArrowGenerator{Name: "fletcher/1", Confidence: "medium"},
		Picks: map[domain.OS]picker.Pick{
			domain.OSLinuxAMD64: {
				Asset: domain.ReleaseAsset{
					Name:   "tool-linux-x86_64.tar.gz",
					URL:    "https://github.com/acme/tool/releases/download/v1/tool-linux-x86_64.tar.gz",
					Digest: fletchDigest,
				},
				Format:    picker.FormatArchive,
				Match:     picker.MatchExact,
				NameMatch: true,
			},
		},
	})
	require.NoError(t, err)
	return data
}

func draftAt(
	t *testing.T,
	good string,
) func(tag string) (gather.Draft, error) {
	manifest := inferredManifest(t)
	return func(tag string) (gather.Draft, error) {
		if tag != good {
			return gather.Draft{}, models.NotFletchableError{Reason: models.ReasonNoReleaseAssets}
		}
		return gather.Draft{Manifest: manifest}, nil
	}
}

func manifestMissing() error {
	return fmt.Errorf("%w: github.com/acme/tool@v1.2.0", resolver.ErrManifestNotFound)
}

func TestRecover_NeverFletchesOnFailure(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		ns   domain.Namespace
	}{
		{name: "transport failure", err: fmt.Errorf("%w: HTTP 503", resolver.ErrFetchFailed), ns: "github.com/acme/tool@v1.2.0"},
		{name: "rate limited", err: fmt.Errorf("%w: HTTP 429", resolver.ErrFetchFailed), ns: "github.com/acme/tool@v1.2.0"},
		{name: "bare not found", err: resolver.ErrNotFound, ns: "github.com/acme/tool@v1.2.0"},
		{name: "unclassified", err: errors.New("boom"), ns: "github.com/acme/tool@v1.2.0"},
		{name: "quiver hosted", err: manifestMissing(), ns: "github.com/acme/tools/tool@v1.2.0"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			drafter := &stubDrafter{answer: draftAt(t, "v1.2.0")}
			f := fallback.New(nil, &stubReleases{}, drafter)

			raw, filename, err := f.Recover(context.Background(), tc.ns, tc.err)

			assert.ErrorIs(t, err, tc.err)
			assert.Nil(t, raw)
			assert.Empty(t, filename)
			assert.Empty(t, drafter.fletchTags)
		})
	}
}

func TestRecover_ManifestNotFound_ForgesInferredArrow(t *testing.T) {
	drafter := &stubDrafter{answer: draftAt(t, "v1.2.0")}
	f := fallback.New(nil, &stubReleases{}, drafter)

	raw, filename, err := f.Recover(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"), manifestMissing())

	require.NoError(t, err)
	assert.Equal(t, []string{"v1.2.0"}, drafter.fletchTags)
	assert.Equal(t, inferredManifest(t), raw)
	assert.Equal(t, "ARROW.md", filename)
}

func TestRecover_TagFallbackChain(t *testing.T) {
	testCases := []struct {
		name     string
		ns       domain.Namespace
		stable   string
		unstable string
		branch   string
		good     string
		wantTags []string
	}{
		{
			name:     "ref release exists",
			ns:       "github.com/acme/tool@v1.2.0",
			stable:   "v2.0.0",
			good:     "v1.2.0",
			wantTags: []string{"v1.2.0"},
		},
		{
			name:     "default branch ref has no release, latest stable does",
			ns:       "github.com/acme/tool@main",
			stable:   "v2.0.0",
			branch:   "main",
			good:     "v2.0.0",
			wantTags: []string{"main", "v2.0.0"},
		},
		{
			name:     "refless goes straight to latest stable",
			ns:       "github.com/acme/tool",
			stable:   "v2.0.0",
			good:     "v2.0.0",
			wantTags: []string{"v2.0.0"},
		},
		{
			name:     "prerelease only falls through to first non-stable channel",
			ns:       "github.com/acme/tool",
			unstable: "v3.0.0-beta.2",
			good:     "v3.0.0-beta.2",
			wantTags: []string{"v3.0.0-beta.2"},
		},
		{
			name:     "default branch ref falls through to first non-stable channel",
			ns:       "github.com/acme/tool@main",
			branch:   "main",
			unstable: "v3.0.0-rc1",
			good:     "v3.0.0-rc1",
			wantTags: []string{"main", "v3.0.0-rc1"},
		},
		{
			name:     "branch ref equal to latest release is tried once",
			ns:       "github.com/acme/tool@main",
			stable:   "main",
			branch:   "main",
			unstable: "v3.0.0-rc1",
			good:     "v3.0.0-rc1",
			wantTags: []string{"main", "v3.0.0-rc1"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			drafter := &stubDrafter{answer: draftAt(t, tc.good)}
			releases := &stubReleases{stable: tc.stable, unstable: tc.unstable, branch: tc.branch}
			f := fallback.New(hostedBy(&stubHost{}), releases, drafter)

			raw, filename, err := f.Recover(context.Background(), tc.ns, manifestMissing())

			require.NoError(t, err)
			assert.Equal(t, tc.wantTags, drafter.fletchTags)
			assert.Equal(t, inferredManifest(t), raw)
			assert.Equal(t, "ARROW.md", filename)
		})
	}
}

func TestRecover_FallbackOnlyForBranchOrEmptyRef(t *testing.T) {
	testCases := []struct {
		name         string
		ns           domain.Namespace
		hostBranches []string
		branch       string
		branchErr    error
		wantTags     []string
		wantErr      bool
	}{
		{
			name:     "exact tag without release never falls back",
			ns:       "github.com/acme/tool@v1.0.0",
			branch:   "main",
			wantTags: []string{"v1.0.0"},
			wantErr:  true,
		},
		{
			name:      "exact tag with unreachable default branch never falls back",
			ns:        "github.com/acme/tool@v1.0.0",
			branchErr: errors.New("ls-remote failed"),
			wantTags:  []string{"v1.0.0"},
			wantErr:   true,
		},
		{
			name:     "empty ref falls back",
			ns:       "github.com/acme/tool",
			wantTags: []string{"v2.0.0"},
		},
		{
			name:     "repository default branch falls back",
			ns:       "github.com/acme/tool@main",
			branch:   "main",
			wantTags: []string{"main", "v2.0.0"},
		},
		{
			name:         "host default branch falls back",
			ns:           "github.com/acme/tool@master",
			hostBranches: []string{"main", "master"},
			branch:       "trunk",
			wantTags:     []string{"master", "v2.0.0"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			drafter := &stubDrafter{answer: draftAt(t, "v2.0.0")}
			releases := &stubReleases{stable: "v2.0.0", branch: tc.branch, branchErr: tc.branchErr}
			f := fallback.New(hostedBy(&stubHost{branches: tc.hostBranches}), releases, drafter)

			_, _, err := f.Recover(context.Background(), tc.ns, manifestMissing())

			assert.Equal(t, tc.wantTags, drafter.fletchTags)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
			var nf models.NotFletchableError
			require.ErrorAs(t, err, &nf)
			assert.Equal(t, models.ReasonNoReleaseAssets, nf.Reason)
			assert.Zero(t, releases.stableCalls)
		})
	}
}

func TestRecover_FailuresSurface(t *testing.T) {
	transient := errors.New("release page: HTTP 503")
	listFailed := errors.New("ls-remote failed")
	testCases := []struct {
		name       string
		answer     func(tag string) (gather.Draft, error)
		unstable   string
		listErr    error
		wantReason models.Reason
		wantErr    error
		wantTags   []string
	}{
		{
			name:       "nothing anywhere",
			answer:     draftAt(t, "none"),
			unstable:   "v2.0.0-rc1",
			wantReason: models.ReasonNoReleaseAssets,
			wantTags:   []string{"main", "v2.0.0-rc1"},
		},
		{
			name:     "channel listing fails",
			answer:   draftAt(t, "none"),
			listErr:  listFailed,
			wantErr:  listFailed,
			wantTags: []string{"main"},
		},
		{
			name: "no usable asset stops the chain",
			answer: func(string) (gather.Draft, error) {
				return gather.Draft{}, models.NotFletchableError{Reason: models.ReasonNoUsableAsset}
			},
			unstable:   "v2.0.0-rc1",
			wantReason: models.ReasonNoUsableAsset,
			wantTags:   []string{"main"},
		},
		{
			name: "low confidence stops the chain",
			answer: func(string) (gather.Draft, error) {
				return gather.Draft{}, models.NotFletchableError{Reason: models.ReasonLowConfidence}
			},
			unstable:   "v2.0.0-rc1",
			wantReason: models.ReasonLowConfidence,
			wantTags:   []string{"main"},
		},
		{
			name: "transient failure stops the chain",
			answer: func(string) (gather.Draft, error) {
				return gather.Draft{}, transient
			},
			unstable: "v2.0.0-rc1",
			wantErr:  transient,
			wantTags: []string{"main"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			drafter := &stubDrafter{answer: tc.answer}
			releases := &stubReleases{unstable: tc.unstable, unstableErr: tc.listErr, branch: "main"}
			f := fallback.New(nil, releases, drafter)

			_, _, err := f.Recover(context.Background(), domain.Namespace("github.com/acme/tool@main"), manifestMissing())

			require.Error(t, err)
			assert.Equal(t, tc.wantTags, drafter.fletchTags)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.NotErrorIs(t, err, resolver.ErrNotFound)
				assert.ErrorIs(t, err, resolver.ErrFetchFailed)
				return
			}
			assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
			assert.NotErrorIs(t, err, resolver.ErrFetchFailed)
			var nf models.NotFletchableError
			require.ErrorAs(t, err, &nf)
			assert.Equal(t, tc.wantReason, nf.Reason)
		})
	}
}

func TestRecover_LookupFailureBeatsNoReleaseAssets(t *testing.T) {
	listFailed := errors.New("ls-remote failed")
	testCases := []struct {
		name       string
		stable     string
		unstable   string
		listErr    error
		wantErr    error
		wantReason models.Reason
		wantTags   []string
	}{
		{
			name:     "stable source ran then channel listing fails",
			stable:   "v2.0.0",
			listErr:  listFailed,
			wantErr:  listFailed,
			wantTags: []string{"v2.0.0"},
		},
		{
			name:       "every source ran and found no assets",
			stable:     "v2.0.0",
			unstable:   "v3.0.0-rc1",
			wantReason: models.ReasonNoReleaseAssets,
			wantTags:   []string{"v2.0.0", "v3.0.0-rc1"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			drafter := &stubDrafter{answer: draftAt(t, "none")}
			releases := &stubReleases{stable: tc.stable, unstable: tc.unstable, unstableErr: tc.listErr}
			f := fallback.New(hostedBy(&stubHost{}), releases, drafter)

			_, _, err := f.Recover(context.Background(), domain.Namespace("github.com/acme/tool"), manifestMissing())

			require.Error(t, err)
			assert.Equal(t, tc.wantTags, drafter.fletchTags)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.NotErrorIs(t, err, models.ErrNotFletchable)
				assert.NotErrorIs(t, err, resolver.ErrNotFound)
				return
			}
			assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
			var nf models.NotFletchableError
			require.ErrorAs(t, err, &nf)
			assert.Equal(t, tc.wantReason, nf.Reason)
		})
	}
}

func TestRecover_NoReleaseTagFletchesNothing(t *testing.T) {
	drafter := &stubDrafter{answer: draftAt(t, "main")}
	f := fallback.New(nil, &stubReleases{branch: "main"}, drafter)

	_, _, err := f.Recover(context.Background(), domain.Namespace("github.com/acme/tool"), manifestMissing())

	assert.ErrorIs(t, err, models.ErrNotFletchable)
	assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
	assert.Empty(t, drafter.fletchTags)
}
