package fallback_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/fallback"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/forge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	manifoldModels "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const fletchDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000001"

type stubDrafter struct {
	answer     func(tag string) ([]byte, error)
	fletchTags []string
}

func (s *stubDrafter) Draft(
	_ context.Context,
	_ domain.Namespace,
	tag string,
) ([]byte, error) {
	s.fletchTags = append(s.fletchTags, tag)
	return s.answer(tag)
}

type stubReleases struct {
	stable      string
	stableErr   error
	unstable    string
	channels    []manifoldModels.ChannelInfo
	unstableErr error
	branch      string
	branchErr   error
	stableCalls int
}

func (s *stubReleases) ResolveLatestStable(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	s.stableCalls++
	return s.stable, s.stableErr
}

func (s *stubReleases) ListChannels(
	_ context.Context,
	_ domain.Namespace,
) ([]manifoldModels.ChannelInfo, error) {
	if s.unstableErr != nil {
		return nil, s.unstableErr
	}
	if s.channels != nil || s.unstable == "" {
		return s.channels, nil
	}
	return []manifoldModels.ChannelInfo{{Name: "beta", Kind: "ordered", Latest: s.unstable}}, nil
}

func (s *stubReleases) ResolveDefaultBranch(
	_ context.Context,
	_ domain.Namespace,
) (string, string, error) {
	return s.branch, "", s.branchErr
}

type stubHost struct {
	hosts.Host
	branches []string
}

func (s *stubHost) DefaultBranches() []string { return s.branches }

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
) func(tag string) ([]byte, error) {
	manifest := inferredManifest(t)
	return func(tag string) ([]byte, error) {
		if tag != good {
			return nil, models.NotFletchableError{Reason: models.ReasonNoReleaseAssets}
		}
		return manifest, nil
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
			f := fallback.New(hosts.None, &stubReleases{}, drafter)

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
	f := fallback.New(hosts.None, &stubReleases{}, drafter)

	raw, filename, err := f.Recover(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"), manifestMissing())

	require.NoError(t, err)
	assert.Equal(t, []string{"v1.2.0"}, drafter.fletchTags)
	assert.Equal(t, inferredManifest(t), raw)
	assert.Equal(t, "ARROW.md", filename)
}

func TestRecover_TagFallbackChain(t *testing.T) {
	testCases := []struct {
		name         string
		ns           domain.Namespace
		stable       string
		unstable     string
		branch       string
		branchErr    error
		hostBranches []string
		good         string
		wantTags     []string
		wantMissing  bool
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
			name:         "host default branch falls back",
			ns:           "github.com/acme/tool@master",
			stable:       "v2.0.0",
			hostBranches: []string{"main", "master"},
			branch:       "trunk",
			good:         "v2.0.0",
			wantTags:     []string{"master", "v2.0.0"},
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
		{
			name:        "exact tag without release never falls back",
			ns:          "github.com/acme/tool@v1.0.0",
			stable:      "v2.0.0",
			branch:      "main",
			good:        "v2.0.0",
			wantTags:    []string{"v1.0.0"},
			wantMissing: true,
		},
		{
			name:        "exact tag with unreachable default branch never falls back",
			ns:          "github.com/acme/tool@v1.0.0",
			stable:      "v2.0.0",
			branchErr:   errors.New("ls-remote failed"),
			good:        "v2.0.0",
			wantTags:    []string{"v1.0.0"},
			wantMissing: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			drafter := &stubDrafter{answer: draftAt(t, tc.good)}
			releases := &stubReleases{stable: tc.stable, unstable: tc.unstable, branch: tc.branch, branchErr: tc.branchErr}
			f := fallback.New(hostedBy(&stubHost{branches: tc.hostBranches}), releases, drafter)

			raw, filename, err := f.Recover(context.Background(), tc.ns, manifestMissing())

			assert.Equal(t, tc.wantTags, drafter.fletchTags)
			if tc.wantMissing {
				assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
				var nf models.NotFletchableError
				require.ErrorAs(t, err, &nf)
				assert.Equal(t, models.ReasonNoReleaseAssets, nf.Reason)
				assert.Zero(t, releases.stableCalls)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, inferredManifest(t), raw)
			assert.Equal(t, "ARROW.md", filename)
		})
	}
}

func TestRecover_FailuresSurface(t *testing.T) {
	transient := errors.New("release page: HTTP 503")
	listFailed := errors.New("ls-remote failed")
	testCases := []struct {
		name       string
		ns         domain.Namespace
		answer     func(tag string) ([]byte, error)
		stable     string
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
			answer: func(string) ([]byte, error) {
				return nil, models.NotFletchableError{Reason: models.ReasonNoUsableAsset}
			},
			unstable:   "v2.0.0-rc1",
			wantReason: models.ReasonNoUsableAsset,
			wantTags:   []string{"main"},
		},
		{
			name: "low confidence stops the chain",
			answer: func(string) ([]byte, error) {
				return nil, models.NotFletchableError{Reason: models.ReasonLowConfidence}
			},
			unstable:   "v2.0.0-rc1",
			wantReason: models.ReasonLowConfidence,
			wantTags:   []string{"main"},
		},
		{
			name: "transient failure stops the chain",
			answer: func(string) ([]byte, error) {
				return nil, transient
			},
			unstable: "v2.0.0-rc1",
			wantErr:  transient,
			wantTags: []string{"main"},
		},
		{
			name:     "channel listing fails after the stable source ran",
			ns:       "github.com/acme/tool",
			answer:   draftAt(t, "none"),
			stable:   "v2.0.0",
			listErr:  listFailed,
			wantErr:  listFailed,
			wantTags: []string{"v2.0.0"},
		},
		{
			name:       "every source ran and found no assets",
			ns:         "github.com/acme/tool",
			answer:     draftAt(t, "none"),
			stable:     "v2.0.0",
			unstable:   "v3.0.0-rc1",
			wantReason: models.ReasonNoReleaseAssets,
			wantTags:   []string{"v2.0.0", "v3.0.0-rc1"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			drafter := &stubDrafter{answer: tc.answer}
			releases := &stubReleases{stable: tc.stable, unstable: tc.unstable, unstableErr: tc.listErr, branch: "main"}
			f := fallback.New(hosts.None, releases, drafter)

			_, _, err := f.Recover(context.Background(), cmp.Or(tc.ns, domain.Namespace("github.com/acme/tool@main")), manifestMissing())

			require.Error(t, err)
			assert.Equal(t, tc.wantTags, drafter.fletchTags)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.NotErrorIs(t, err, resolver.ErrNotFound)
				assert.NotErrorIs(t, err, models.ErrNotFletchable)
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

func TestRecover_NoReleaseTagFletchesNothing(t *testing.T) {
	drafter := &stubDrafter{answer: draftAt(t, "main")}
	f := fallback.New(hosts.None, &stubReleases{branch: "main"}, drafter)

	_, _, err := f.Recover(context.Background(), domain.Namespace("github.com/acme/tool"), manifestMissing())

	assert.ErrorIs(t, err, models.ErrNotFletchable)
	assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
	assert.Empty(t, drafter.fletchTags)
}

func TestRecover_ReleaseLookupPolicy(t *testing.T) {
	boom := errors.New("lookup failed")
	testCases := []struct {
		name     string
		releases *stubReleases
		wantTags []string
		wantErr  error
	}{
		{
			name:     "no latest stable is a miss",
			releases: &stubReleases{stableErr: fmt.Errorf("wrap: %w", manifoldModels.ErrNoLatestStable), unstable: "v3.0.0-rc1"},
			wantTags: []string{"v3.0.0-rc1"},
		},
		{
			name:     "no tag in channel is a miss",
			releases: &stubReleases{stable: "v2.0.0", unstableErr: fmt.Errorf("wrap: %w", manifoldModels.ErrNoTagInChannel)},
			wantTags: []string{"v2.0.0"},
		},
		{
			name:     "other stable lookup failure surfaces",
			releases: &stubReleases{stableErr: boom},
			wantErr:  boom,
		},
		{
			name: "stable and default branch fallback channels are skipped",
			releases: &stubReleases{channels: []manifoldModels.ChannelInfo{
				{Name: "stable", Latest: "v2.0.0"},
				{Name: "main", Latest: "main", IsDefaultBranchFallback: true},
				{Name: "beta", Latest: "v3.0.0-beta.2"},
				{Name: "nightly", Latest: "nightly"},
			}},
			wantTags: []string{"v3.0.0-beta.2"},
		},
		{
			name: "only skipped channels is a miss",
			releases: &stubReleases{channels: []manifoldModels.ChannelInfo{
				{Name: "stable", Latest: "v2.0.0"},
				{Name: "main", Latest: "main", IsDefaultBranchFallback: true},
			}},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			drafter := &stubDrafter{answer: draftAt(t, "none")}
			f := fallback.New(hosts.None, tc.releases, drafter)

			_, _, err := f.Recover(context.Background(), domain.Namespace("github.com/acme/tool"), manifestMissing())

			require.Error(t, err)
			assert.Equal(t, tc.wantTags, drafter.fletchTags)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
		})
	}
}
