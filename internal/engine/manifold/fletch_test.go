package manifold

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/forge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const fletchDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000001"

type stubFletcher struct {
	answer     func(tag string) (fletcher.Draft, error)
	fletchTags []string
	probeTags  []string
	probeHints []fletcher.Hint
}

func (s *stubFletcher) Fletch(
	_ context.Context,
	_ domain.Namespace,
	tag string,
) (fletcher.Draft, error) {
	s.fletchTags = append(s.fletchTags, tag)
	return s.answer(tag)
}

func (s *stubFletcher) Probe(
	_ context.Context,
	_ domain.Namespace,
	tag string,
	hint fletcher.Hint,
) (fletcher.Draft, error) {
	s.probeTags = append(s.probeTags, tag)
	s.probeHints = append(s.probeHints, hint)
	return s.answer(tag)
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
				Asset:     hostAsset("tool-linux-x86_64.tar.gz"),
				Format:    picker.FormatArchive,
				Match:     picker.MatchExact,
				NameMatch: true,
			},
		},
	})
	require.NoError(t, err)
	return data
}

func hostAsset(
	name string,
) Asset {
	return Asset{
		Name:   name,
		URL:    "https://github.com/acme/tool/releases/download/v1/" + name,
		Digest: fletchDigest,
	}
}

func draftAt(
	t *testing.T,
	good string,
) func(tag string) (fletcher.Draft, error) {
	manifest := inferredManifest(t)
	return func(tag string) (fletcher.Draft, error) {
		if tag != good {
			return fletcher.Draft{}, fletcher.NotFletchableError{Reason: fletcher.ReasonNoReleaseAssets}
		}
		return fletcher.Draft{Manifest: manifest}, nil
	}
}

func manifestMissing() error {
	return fmt.Errorf("%w: github.com/acme/tool@v1.2.0", resolver.ErrManifestNotFound)
}

func TestResolveArrow_FletcherDisabled_KeepsManifestNotFound(t *testing.T) {
	m := NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil)

	_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

	assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
	assert.ErrorIs(t, err, resolver.ErrNotFound)
	assert.NotErrorIs(t, err, fletcher.ErrNotFletchable)
}

func TestResolveArrow_FletcherEnabled_DeclaredManifestWins(t *testing.T) {
	fl := &stubFletcher{answer: draftAt(t, "v1.2.0")}
	declared := inferredManifest(t)
	m := NewWithResolvers(
		&stubResolver{arrowData: declared, arrowFilename: "arrow.yaml"},
		&stubConstraintResolver{},
		nil,
		WithFletcher(fl),
	)

	_, raw, filename, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

	require.NoError(t, err)
	assert.Equal(t, declared, raw)
	assert.Equal(t, "arrow.yaml", filename)
	assert.Empty(t, fl.fletchTags)
}

func TestResolveArrow_FletcherEnabled_NeverFletchesOnFailure(t *testing.T) {
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
			fl := &stubFletcher{answer: draftAt(t, "v1.2.0")}
			m := NewWithResolvers(
				&stubResolver{arrowErr: tc.err, quiverErr: tc.err},
				&stubConstraintResolver{},
				nil,
				WithFletcher(fl),
			)

			_, _, _, err := m.ResolveArrow(context.Background(), tc.ns)

			assert.ErrorIs(t, err, tc.err)
			assert.Empty(t, fl.fletchTags)
		})
	}
}

func TestResolveArrow_FletcherEnabled_ManifestNotFound_ForgesInferredArrow(t *testing.T) {
	fl := &stubFletcher{answer: draftAt(t, "v1.2.0")}
	m := NewWithResolvers(
		&stubResolver{arrowErr: manifestMissing()},
		&stubConstraintResolver{},
		nil,
		WithFletcher(fl),
	)

	arrow, raw, filename, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

	require.NoError(t, err)
	assert.Equal(t, []string{"v1.2.0"}, fl.fletchTags)
	assert.Equal(t, inferredManifest(t), raw)
	assert.Equal(t, "ARROW.md", filename)
	assert.Equal(t, domain.ArrowOriginInferred, arrow.Origin())
	assert.Equal(t, "tool", arrow.Name)
}

func TestResolveDeclaredArrow_FletcherEnabled_NeverFletchesOnManifestNotFound(t *testing.T) {
	fl := &stubFletcher{answer: draftAt(t, "v1.2.0")}
	m := NewWithResolvers(
		&stubResolver{arrowErr: manifestMissing()},
		&stubConstraintResolver{},
		nil,
		WithFletcher(fl),
	)

	_, _, _, err := m.ResolveDeclaredArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

	assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
	assert.Empty(t, fl.fletchTags)
	assert.Empty(t, fl.probeTags)
}

func TestResolveDeclaredArrow_FletcherEnabled_DeclaredManifestStillResolves(t *testing.T) {
	declared := inferredManifest(t)
	m := NewWithResolvers(
		&stubResolver{arrowData: declared, arrowFilename: "arrow.yaml"},
		&stubConstraintResolver{},
		nil,
		WithFletcher(&stubFletcher{answer: draftAt(t, "v1.2.0")}),
	)

	arrow, raw, filename, err := m.ResolveDeclaredArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

	require.NoError(t, err)
	assert.Equal(t, declared, raw)
	assert.Equal(t, "arrow.yaml", filename)
	assert.NotNil(t, arrow)
}

func TestResolveArrow_FletcherEnabled_TagFallbackChain(t *testing.T) {
	testCases := []struct {
		name     string
		ns       domain.Namespace
		release  string
		branch   string
		tags     []string
		good     string
		wantTags []string
	}{
		{
			name:     "ref release exists",
			ns:       "github.com/acme/tool@v1.2.0",
			release:  "v2.0.0",
			good:     "v1.2.0",
			wantTags: []string{"v1.2.0"},
		},
		{
			name:     "default branch ref has no release, latest stable does",
			ns:       "github.com/acme/tool@main",
			release:  "v2.0.0",
			branch:   "main",
			good:     "v2.0.0",
			wantTags: []string{"main", "v2.0.0"},
		},
		{
			name:     "refless goes straight to latest stable",
			ns:       "github.com/acme/tool",
			release:  "v2.0.0",
			good:     "v2.0.0",
			wantTags: []string{"v2.0.0"},
		},
		{
			name:     "prerelease only falls through to first non-stable channel",
			ns:       "github.com/acme/tool",
			tags:     []string{"v3.0.0-beta.1", "v3.0.0-beta.2", "nightly"},
			good:     "v3.0.0-beta.2",
			wantTags: []string{"v3.0.0-beta.2"},
		},
		{
			name:     "default branch ref falls through to first non-stable channel",
			ns:       "github.com/acme/tool@main",
			branch:   "main",
			tags:     []string{"v3.0.0-rc1"},
			good:     "v3.0.0-rc1",
			wantTags: []string{"main", "v3.0.0-rc1"},
		},
		{
			name:     "branch ref equal to latest release is tried once",
			ns:       "github.com/acme/tool@main",
			release:  "main",
			branch:   "main",
			tags:     []string{"v2.0.0", "v3.0.0-rc1"},
			good:     "v3.0.0-rc1",
			wantTags: []string{"main", "v3.0.0-rc1"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fl := &stubFletcher{answer: draftAt(t, tc.good)}
			m := NewWithResolvers(
				&stubResolver{arrowErr: manifestMissing()},
				&stubConstraintResolver{err: errors.New("no tags"), listTags: tc.tags, branch: tc.branch},
				hostedBy(&stubHost{ref: tc.release}),
				WithFletcher(fl),
			)

			arrow, _, _, err := m.ResolveArrow(context.Background(), tc.ns)

			require.NoError(t, err)
			assert.Equal(t, tc.wantTags, fl.fletchTags)
			assert.Equal(t, domain.ArrowOriginInferred, arrow.Origin())
		})
	}
}

func TestResolveArrow_FletcherEnabled_FallbackOnlyForBranchOrEmptyRef(t *testing.T) {
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
			fl := &stubFletcher{answer: draftAt(t, "v2.0.0")}
			host := &stubHost{ref: "v2.0.0", branches: tc.hostBranches}
			m := NewWithResolvers(
				&stubResolver{arrowErr: manifestMissing()},
				&stubConstraintResolver{err: errors.New("no tags"), branch: tc.branch, branchErr: tc.branchErr},
				hostedBy(host),
				WithFletcher(fl),
			)

			_, _, _, err := m.ResolveArrow(context.Background(), tc.ns)

			assert.Equal(t, tc.wantTags, fl.fletchTags)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			var nf fletcher.NotFletchableError
			require.ErrorAs(t, err, &nf)
			assert.Equal(t, fletcher.ReasonNoReleaseAssets, nf.Reason)
			assert.Zero(t, host.called)
		})
	}
}

func TestResolveArrow_FletcherEnabled_FailuresSurface(t *testing.T) {
	transient := errors.New("release page: HTTP 503")
	listFailed := errors.New("ls-remote failed")
	testCases := []struct {
		name       string
		answer     func(tag string) (fletcher.Draft, error)
		channels   []string
		listErr    error
		wantReason string
		wantErr    error
		wantTags   []string
	}{
		{
			name:       "nothing anywhere",
			answer:     draftAt(t, "none"),
			channels:   []string{"v1.0.0", "v2.0.0-rc1"},
			wantReason: fletcher.ReasonNoReleaseAssets,
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
			answer: func(string) (fletcher.Draft, error) {
				return fletcher.Draft{}, fletcher.NotFletchableError{Reason: fletcher.ReasonNoUsableAsset}
			},
			channels:   []string{"v2.0.0-rc1"},
			wantReason: fletcher.ReasonNoUsableAsset,
			wantTags:   []string{"main"},
		},
		{
			name: "transient failure stops the chain",
			answer: func(string) (fletcher.Draft, error) {
				return fletcher.Draft{}, transient
			},
			channels: []string{"v2.0.0-rc1"},
			wantErr:  transient,
			wantTags: []string{"main"},
		},
		{
			name: "forged bytes that do not parse",
			answer: func(string) (fletcher.Draft, error) {
				return fletcher.Draft{Manifest: []byte("not: [a manifest")}, nil
			},
			wantErr:  ErrInvalidManifest,
			wantTags: []string{"main"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fl := &stubFletcher{answer: tc.answer}
			m := NewWithResolvers(
				&stubResolver{arrowErr: manifestMissing()},
				&stubConstraintResolver{err: errors.New("no tags"), listTags: tc.channels, listTagsErr: tc.listErr, branch: "main"},
				nil,
				WithFletcher(fl),
			)

			_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@main"))

			require.Error(t, err)
			assert.Equal(t, tc.wantTags, fl.fletchTags)
			assert.NotErrorIs(t, err, resolver.ErrNotFound)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Equal(t, tc.wantErr != ErrInvalidManifest, errors.Is(err, resolver.ErrFetchFailed))
				return
			}
			assert.NotErrorIs(t, err, resolver.ErrFetchFailed)
			var nf fletcher.NotFletchableError
			require.ErrorAs(t, err, &nf)
			assert.Equal(t, tc.wantReason, nf.Reason)
		})
	}
}

func TestResolveArrow_FletcherEnabled_LookupFailureBeatsNoReleaseAssets(t *testing.T) {
	listFailed := errors.New("ls-remote failed")
	testCases := []struct {
		name       string
		release    string
		channels   []string
		listErr    error
		wantErr    error
		wantReason string
		wantTags   []string
	}{
		{
			name:     "stable source ran then channel listing fails",
			release:  "v2.0.0",
			listErr:  listFailed,
			wantErr:  listFailed,
			wantTags: []string{"v2.0.0"},
		},
		{
			name:       "every source ran and found no assets",
			release:    "v2.0.0",
			channels:   []string{"v3.0.0-rc1"},
			wantReason: fletcher.ReasonNoReleaseAssets,
			wantTags:   []string{"v2.0.0", "v3.0.0-rc1"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fl := &stubFletcher{answer: draftAt(t, "none")}
			m := NewWithResolvers(
				&stubResolver{arrowErr: manifestMissing()},
				&stubConstraintResolver{err: errors.New("no tags"), listTags: tc.channels, listTagsErr: tc.listErr},
				hostedBy(&stubHost{ref: tc.release}),
				WithFletcher(fl),
			)

			_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool"))

			require.Error(t, err)
			assert.Equal(t, tc.wantTags, fl.fletchTags)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.NotErrorIs(t, err, fletcher.ErrNotFletchable)
				return
			}
			var nf fletcher.NotFletchableError
			require.ErrorAs(t, err, &nf)
			assert.Equal(t, tc.wantReason, nf.Reason)
		})
	}
}

func TestFetchFailure(t *testing.T) {
	transient := errors.New("repo page: HTTP 503")
	alreadyFailed := fmt.Errorf("readme: %w", resolver.ErrFetchFailed)
	notFletchable := fletcher.NotFletchableError{Reason: fletcher.ReasonNoUsableAsset}

	testCases := []struct {
		name string
		err  error
		want string
	}{
		{name: "transient error becomes a fetch failure", err: transient, want: "resolver: fetch failed: repo page: HTTP 503"},
		{name: "fetch failure is kept as is", err: alreadyFailed, want: alreadyFailed.Error()},
		{name: "not fletchable is kept as is", err: notFletchable, want: notFletchable.Error()},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := fetchFailure(tc.err)

			assert.ErrorIs(t, got, tc.err)
			assert.Equal(t, tc.want, got.Error())
		})
	}
}

func TestLookupFailure(t *testing.T) {
	boom := errors.New("boom")
	testCases := []struct {
		name string
		err  error
		want error
	}{
		{name: "nil", err: nil, want: nil},
		{name: "no latest stable is a miss", err: fmt.Errorf("wrap: %w", ErrNoLatestStable), want: nil},
		{name: "no tag in channel is a miss", err: fmt.Errorf("wrap: %w", ErrNoTagInChannel), want: nil},
		{name: "anything else is a failure", err: boom, want: boom},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, lookupFailure(tc.err))
		})
	}
}

func TestResolveArrow_FletcherEnabled_SkipsDefaultBranchFallbackChannel(t *testing.T) {
	fl := &stubFletcher{answer: draftAt(t, "main")}
	m := NewWithResolvers(
		&stubResolver{arrowErr: manifestMissing()},
		&stubConstraintResolver{err: errors.New("no tags"), branch: "main"},
		nil,
		WithFletcher(fl),
	)

	_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool"))

	assert.ErrorIs(t, err, fletcher.ErrNotFletchable)
	assert.Empty(t, fl.fletchTags)
}

func TestProbeArrow_Disabled_IsNotFletchable(t *testing.T) {
	m := NewWithResolvers(&stubResolver{}, &stubConstraintResolver{}, nil)

	arrow, raw, err := m.ProbeArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"), fletcher.Hint{})

	assert.Nil(t, arrow)
	assert.Nil(t, raw)
	var nf fletcher.NotFletchableError
	require.ErrorAs(t, err, &nf)
	assert.Equal(t, fletcher.ReasonDisabled, nf.Reason)
	assert.Contains(t, err.Error(), "manifold: probe github.com/acme/tool@v1.2.0")
}

func TestProbeArrow_QuiverHosted_IsHostUnsupported(t *testing.T) {
	fl := &stubFletcher{answer: draftAt(t, "v1.2.0")}
	m := NewWithResolvers(&stubResolver{}, &stubConstraintResolver{}, nil, WithFletcher(fl))

	arrow, raw, err := m.ProbeArrow(context.Background(), domain.Namespace("github.com/acme/tools/tool@v1.2.0"), fletcher.Hint{})

	assert.Nil(t, arrow)
	assert.Nil(t, raw)
	var nf fletcher.NotFletchableError
	require.ErrorAs(t, err, &nf)
	assert.Equal(t, fletcher.ReasonHostUnsupported, nf.Reason)
	assert.Empty(t, fl.probeTags)
}

func TestProbeArrow_Enabled_ParsesProbeDraft(t *testing.T) {
	fl := &stubFletcher{answer: draftAt(t, "v2.0.0")}
	m := NewWithResolvers(
		&stubResolver{},
		&stubConstraintResolver{},
		hostedBy(&stubHost{ref: "v2.0.0"}),
		WithFletcher(fl),
	)
	hint := fletcher.Hint{Name: "Tool", Description: "does things"}

	arrow, raw, err := m.ProbeArrow(context.Background(), domain.Namespace("github.com/acme/tool"), hint)

	require.NoError(t, err)
	assert.Equal(t, inferredManifest(t), raw)
	assert.Equal(t, domain.ArrowOriginInferred, arrow.Origin())
	assert.Equal(t, []string{"v2.0.0"}, fl.probeTags)
	assert.Equal(t, []fletcher.Hint{hint}, fl.probeHints)
	assert.Empty(t, fl.fletchTags)
	assert.Equal(t, domain.Namespace("github.com/acme/tool@v2.0.0"), arrow.Namespace)
}

func TestProbeArrow_Enabled_ExplicitRef_SetsNamespaceToThatRef(t *testing.T) {
	fl := &stubFletcher{answer: draftAt(t, "v1.2.0")}
	m := NewWithResolvers(
		&stubResolver{},
		&stubConstraintResolver{},
		nil,
		WithFletcher(fl),
	)

	arrow, _, err := m.ProbeArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"), fletcher.Hint{})

	require.NoError(t, err)
	assert.Equal(t, []string{"v1.2.0"}, fl.probeTags)
	assert.Equal(t, domain.Namespace("github.com/acme/tool@v1.2.0"), arrow.Namespace)
}

func TestProbeArrow_Enabled_Failures(t *testing.T) {
	testCases := []struct {
		name    string
		answer  func(tag string) (fletcher.Draft, error)
		wantErr error
	}{
		{
			name: "not fletchable",
			answer: func(string) (fletcher.Draft, error) {
				return fletcher.Draft{}, fletcher.NotFletchableError{Reason: fletcher.ReasonNoUsableAsset}
			},
			wantErr: fletcher.ErrNotFletchable,
		},
		{
			name: "unparseable draft",
			answer: func(string) (fletcher.Draft, error) {
				return fletcher.Draft{Manifest: []byte("not: [a manifest")}, nil
			},
			wantErr: ErrInvalidManifest,
		},
		{
			name: "transient host failure is a fetch failure",
			answer: func(string) (fletcher.Draft, error) {
				return fletcher.Draft{}, errors.New("expanded assets: HTTP 429")
			},
			wantErr: resolver.ErrFetchFailed,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewWithResolvers(
				&stubResolver{},
				&stubConstraintResolver{},
				nil,
				WithFletcher(&stubFletcher{answer: tc.answer}),
			)

			arrow, raw, err := m.ProbeArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"), fletcher.Hint{})

			assert.ErrorIs(t, err, tc.wantErr)
			assert.Contains(t, err.Error(), "manifold: probe github.com/acme/tool@v1.2.0")
			assert.Nil(t, arrow)
			assert.Nil(t, raw)
		})
	}
}
