package fletcher_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

const (
	testNS  = domain.Namespace("github.com/acme/tool@v1.0.0")
	testTag = "v1.0.0"
	digestA = "sha256:00000000000000000000000000000000000000000000000000000000000000aa"
	digestB = "sha256:00000000000000000000000000000000000000000000000000000000000000bb"
	digestC = "sha256:00000000000000000000000000000000000000000000000000000000000000cc"
)

var errBoom = errors.New("boom")

type plainHost struct{}

func (plainHost) RawFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", nil
}

func (plainHost) DefaultBranches() []string { return nil }

func (plainHost) LatestRelease(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "", nil
}

type stubForge struct {
	plainHost
	assets    []hosts.Asset
	assetsErr error
	page      hosts.RepoPage
	pageErr   error
	files     map[string][]byte
	fileErrs  map[string]error
	assetCall int
	pageCall  int
	rawCalls  []string
}

func (s *stubForge) ReleaseAssets(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) ([]hosts.Asset, error) {
	s.assetCall++
	return s.assets, s.assetsErr
}

func (s *stubForge) RepoPage(
	_ context.Context,
	_ domain.Namespace,
) (hosts.RepoPage, error) {
	s.pageCall++
	return s.page, s.pageErr
}

func (s *stubForge) RawFile(
	_ context.Context,
	_ domain.Namespace,
	_ string,
	path string,
) ([]byte, error) {
	s.rawCalls = append(s.rawCalls, path)
	if err, ok := s.fileErrs[path]; ok {
		return nil, err
	}
	if data, ok := s.files[path]; ok {
		return data, nil
	}
	return nil, hosts.ErrRawNotFound
}

type stubPicker struct {
	picks map[domain.OS]picker.Pick
	repos []string
}

func (s *stubPicker) Pick(
	repo string,
	_ []hosts.Asset,
	platform domain.OS,
) (picker.Pick, bool) {
	s.repos = append(s.repos, repo)
	pick, ok := s.picks[platform]
	return pick, ok
}

func lookupOf(
	host hosts.Host,
) hosts.Lookup {
	return func(_ domain.Namespace) (hosts.Host, bool) {
		return host, true
	}
}

func realAssets() []hosts.Asset {
	return []hosts.Asset{
		{Name: "tool-1.0.0-x86_64-unknown-linux-musl.tar.gz", URL: "https://example.test/linux.tar.gz", Digest: digestA},
		{Name: "tool-1.0.0-aarch64-apple-darwin.tar.gz", URL: "https://example.test/darwin.tar.gz", Digest: digestB},
		{Name: "tool-1.0.0-x86_64-pc-windows-msvc.zip", URL: "https://example.test/windows.zip", Digest: digestC},
	}
}

func exactPick(
	name string,
	format picker.Format,
) picker.Pick {
	return picker.Pick{
		Asset:     hosts.Asset{Name: name, URL: "https://example.test/" + name, Digest: digestA},
		Format:    format,
		Match:     picker.MatchExact,
		NameMatch: true,
	}
}

func parse(
	t *testing.T,
	data []byte,
) *domain.Arrow {
	t.Helper()
	arrow, err := manifold.NewWithResolvers(nil, nil, nil).ParseArrow(data)
	require.NoError(t, err, string(data))
	return arrow
}

func notFletchableReason(
	t *testing.T,
	err error,
) string {
	t.Helper()
	require.ErrorIs(t, err, fletcher.ErrNotFletchable)
	var nf fletcher.NotFletchableError
	require.ErrorAs(t, err, &nf)
	return nf.Reason
}

func TestFletcher_Fletch_ForgesParseableManifest(t *testing.T) {
	host := &stubForge{
		assets: realAssets(),
		page: hosts.RepoPage{
			Description: "Tool searches things.",
			OwnerIsOrg:  true,
			OwnerAvatar: "https://avatars.example.test/acme.png",
		},
		files: map[string][]byte{
			"README.md": []byte("# tool\n\nTool searches things fast.\n\nSee [docs](docs/guide.md).\n"),
		},
	}
	f := fletcher.New(lookupOf(host), picker.New())

	draft, err := f.Fletch(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, draft.Manifest)
	assert.Equal(t, "tool", arrow.Name)
	assert.Equal(t, "Tool searches things.", arrow.Description)
	assert.Equal(t, "https://github.com/acme/tool", arrow.URL)
	assert.Equal(t, "https://avatars.example.test/acme.png", arrow.Media.Icon)
	assert.Contains(t, arrow.Readme, "Tool searches things fast.")
	assert.Contains(t, arrow.Readme, "https://github.com/acme/tool/blob/v1.0.0/docs/guide.md")
	assert.Equal(t, domain.ArrowOriginInferred, arrow.Origin())
	assert.Equal(t, "fletcher/1", arrow.Generator.Name)
	assert.Equal(t, string(draft.Report.Confidence), arrow.Generator.Confidence)
	assert.Equal(t, draft.Report.Warnings, arrow.Generator.Warnings)
	assert.Equal(t, "fletcher/1", draft.Report.Heuristics)
	assert.Contains(t, arrow.Targets, domain.OSLinuxAMD64)
	assert.Contains(t, arrow.Targets, domain.OSDarwinARM64)
	assert.Contains(t, arrow.Targets, domain.OSWindowsAMD64)
	assert.NotContains(t, arrow.Targets, domain.OSLinuxARM64)
	assert.Equal(t,
		[]domain.ExposeEntry{{Name: "tool", Path: domain.ExposeAuto}},
		arrow.Targets[domain.OSLinuxAMD64].Expose.CLI,
	)
	assert.Equal(t, 1, host.assetCall)
	assert.Equal(t, 1, host.pageCall)
}

func TestFletcher_Fletch_ReparseKeepsGenerator(t *testing.T) {
	host := &stubForge{assets: realAssets(), page: hosts.RepoPage{Description: "d"}}
	f := fletcher.New(lookupOf(host), picker.New())
	draft, err := f.Fletch(context.Background(), testNS, testTag)
	require.NoError(t, err)

	first := parse(t, draft.Manifest)
	second := parse(t, draft.Manifest)

	assert.Equal(t, first.Generator, second.Generator)
	assert.Equal(t, domain.ArrowOriginInferred, second.Origin())
	assert.NotEmpty(t, second.Generator.Confidence)
}

func TestFletcher_Fletch_PassesRepoNameToPicker(t *testing.T) {
	pk := &stubPicker{picks: map[domain.OS]picker.Pick{
		domain.OSLinuxAMD64: exactPick("tool-linux.tar.gz", picker.FormatArchive),
	}}
	host := &stubForge{assets: realAssets()}

	_, err := fletcher.New(lookupOf(host), pk).Fletch(context.Background(), testNS, testTag)

	require.NoError(t, err)
	require.Len(t, pk.repos, len(domain.AllOS()))
	for _, repo := range pk.repos {
		assert.Equal(t, "tool", repo)
	}
}

func TestFletcher_NotFletchable(t *testing.T) {
	testCases := []struct {
		name   string
		lookup hosts.Lookup
		picks  map[domain.OS]picker.Pick
		want   string
	}{
		{
			name:   "unknown host",
			lookup: hosts.None,
			want:   fletcher.ReasonHostUnsupported,
		},
		{
			name:   "nil lookup",
			lookup: nil,
			want:   fletcher.ReasonHostUnsupported,
		},
		{
			name:   "host without forge",
			lookup: lookupOf(plainHost{}),
			want:   fletcher.ReasonHostUnsupported,
		},
		{
			name:   "release not found",
			lookup: lookupOf(&stubForge{assetsErr: hosts.ErrReleaseNotFound}),
			want:   fletcher.ReasonNoReleaseAssets,
		},
		{
			name:   "release without assets",
			lookup: lookupOf(&stubForge{}),
			want:   fletcher.ReasonNoReleaseAssets,
		},
		{
			name:   "only unusable assets picked",
			lookup: lookupOf(&stubForge{assets: realAssets()}),
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: {Asset: hosts.Asset{Name: "tool.tar.gz", URL: "https://example.test/tool.tar.gz"}},
				domain.OSLinuxARM64: {Asset: hosts.Asset{Name: "x", URL: "https://example.test/..", Digest: digestA}},
			},
			want: fletcher.ReasonNoUsableAsset,
		},
		{
			name:   "no asset picked",
			lookup: lookupOf(&stubForge{assets: realAssets()}),
			picks:  map[domain.OS]picker.Pick{},
			want:   fletcher.ReasonNoUsableAsset,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := fletcher.New(tc.lookup, &stubPicker{picks: tc.picks})

			_, fletchErr := f.Fletch(context.Background(), testNS, testTag)
			_, probeErr := f.Probe(context.Background(), testNS, testTag, fletcher.Hint{})

			assert.Equal(t, tc.want, notFletchableReason(t, fletchErr))
			assert.Equal(t, tc.want, notFletchableReason(t, probeErr))
		})
	}
}

func TestFletcher_Fletch_HostErrorsPropagate(t *testing.T) {
	cjk := []byte("这是一个非常快速的搜索工具,支持正则表达式")
	testCases := []struct {
		name string
		host *stubForge
	}{
		{
			name: "release assets",
			host: &stubForge{assetsErr: errBoom},
		},
		{
			name: "repo page",
			host: &stubForge{assets: realAssets(), pageErr: errBoom},
		},
		{
			name: "default readme",
			host: &stubForge{assets: realAssets(), fileErrs: map[string]error{"readme.md": errBoom}},
		},
		{
			name: "english readme",
			host: &stubForge{
				assets:   realAssets(),
				files:    map[string][]byte{"README.md": cjk},
				fileErrs: map[string]error{"README_EN.md": errBoom},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := fletcher.New(lookupOf(tc.host), picker.New())

			_, err := f.Fletch(context.Background(), testNS, testTag)

			require.ErrorIs(t, err, errBoom)
			assert.NotErrorIs(t, err, fletcher.ErrNotFletchable)
		})
	}
}

func TestFletcher_Probe_SkipsPageReadmeAndMedia(t *testing.T) {
	host := &stubForge{
		assets: realAssets(),
		files:  map[string][]byte{"README.md": []byte("never read")},
	}
	f := fletcher.New(lookupOf(host), picker.New())

	draft, err := f.Probe(context.Background(), testNS, testTag, fletcher.Hint{
		Name:        "Tool",
		Description: "Discovered tool.",
	})
	require.NoError(t, err)

	arrow := parse(t, draft.Manifest)
	assert.Equal(t, 1, host.assetCall)
	assert.Zero(t, host.pageCall)
	assert.Empty(t, host.rawCalls)
	assert.Equal(t, "Tool", arrow.Name)
	assert.Equal(t, "Discovered tool.", arrow.Description)
	assert.Equal(t, "Discovered tool.", arrow.Readme)
	assert.Equal(t, "https://github.com/acme/tool", arrow.URL)
	assert.Equal(t, domain.ArrowMedia{}, arrow.Media)
	assert.Equal(t, domain.ArrowOriginInferred, arrow.Origin())
	assert.Equal(t,
		[]domain.ExposeEntry{{Name: "tool", Path: domain.ExposeAuto}},
		arrow.Targets[domain.OSLinuxAMD64].Expose.CLI,
	)
}

func TestFletcher_Probe_DropsUnusablePicks(t *testing.T) {
	pk := &stubPicker{picks: map[domain.OS]picker.Pick{
		domain.OSLinuxAMD64: exactPick("tool-linux.tar.gz", picker.FormatArchive),
		domain.OSLinuxARM64: withMatch(
			picker.Pick{Asset: hosts.Asset{Name: "x", URL: "https://example.test/%24%7BHOME%7D", Digest: digestA}},
			picker.MatchAssumed,
			false,
		),
	}}

	draft, err := fletcher.New(lookupOf(&stubForge{assets: realAssets()}), pk).Probe(context.Background(), testNS, testTag, fletcher.Hint{})
	require.NoError(t, err)

	arrow := parse(t, draft.Manifest)
	assert.Contains(t, arrow.Targets, domain.OSLinuxAMD64)
	assert.NotContains(t, arrow.Targets, domain.OSLinuxARM64)
	assert.Equal(t, fletcher.ConfidenceHigh, draft.Report.Confidence)
	assert.Empty(t, draft.Report.Warnings)
}

func TestFletcher_Probe_URLUsesNamespaceHost(t *testing.T) {
	host := &stubForge{assets: realAssets()}

	draft, err := fletcher.New(lookupOf(host), picker.New()).Probe(
		context.Background(),
		domain.Namespace("gitlab.example.com/acme/tool@v1.0.0"),
		testTag,
		fletcher.Hint{},
	)
	require.NoError(t, err)

	assert.Equal(t, "https://gitlab.example.com/acme/tool", parse(t, draft.Manifest).URL)
}

func TestFletcher_Probe_NameFallsBackToRepo(t *testing.T) {
	host := &stubForge{assets: realAssets()}
	f := fletcher.New(lookupOf(host), picker.New())

	draft, err := f.Probe(context.Background(), testNS, testTag, fletcher.Hint{})
	require.NoError(t, err)

	arrow := parse(t, draft.Manifest)
	assert.Equal(t, "tool", arrow.Name)
	assert.Equal(t, "tool", arrow.Readme)
}

func TestFletcher_Probe_HostErrorPropagates(t *testing.T) {
	host := &stubForge{assetsErr: errBoom}

	_, err := fletcher.New(lookupOf(host), picker.New()).Probe(context.Background(), testNS, testTag, fletcher.Hint{})

	require.ErrorIs(t, err, errBoom)
	assert.NotErrorIs(t, err, fletcher.ErrNotFletchable)
}
