package gather_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/confidence"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/gather"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/media"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const (
	testNS  = domain.Namespace("github.com/acme/tool@v1.0.0")
	testTag = "v1.0.0"
	digestA = "sha256:00000000000000000000000000000000000000000000000000000000000000aa"
	digestB = "sha256:00000000000000000000000000000000000000000000000000000000000000bb"
	digestC = "sha256:00000000000000000000000000000000000000000000000000000000000000cc"
)

const (
	testTimeout  = 5 * time.Second
	repoPagePath = "/acme/tool"
)

var errBoom = errors.New("boom")

type stubHost struct {
	mu         sync.Mutex
	server     *httptest.Server
	assets     []domain.ReleaseAsset
	assetsErr  error
	page       string
	pageStatus int
	files      map[string][]byte
	fileStatus map[string]int
	images     map[string][]byte
	onRequest  func(r *http.Request) bool
	assetsWait chan struct{}
	assetCall  int
	pageCall   int
	rawCalls   []string
}

func (s *stubHost) start(
	t *testing.T,
) {
	t.Helper()
	s.server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.server.Close)
}

func (s *stubHost) serve(
	w http.ResponseWriter,
	r *http.Request,
) {
	if s.onRequest != nil && !s.onRequest(r) {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if r.URL.Path == repoPagePath {
		s.servePage(w)
		return
	}
	if data, ok := s.images[r.URL.Path]; ok {
		_, _ = w.Write(data)
		return
	}
	path, ok := strings.CutPrefix(r.URL.Path, "/raw/"+testTag+"/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveFile(w, r, path)
}

func (s *stubHost) servePage(
	w http.ResponseWriter,
) {
	s.mu.Lock()
	s.pageCall++
	s.mu.Unlock()
	if s.pageStatus != 0 {
		w.WriteHeader(s.pageStatus)
		return
	}
	_, _ = w.Write([]byte(s.page))
}

func (s *stubHost) serveFile(
	w http.ResponseWriter,
	r *http.Request,
	path string,
) {
	s.mu.Lock()
	s.rawCalls = append(s.rawCalls, path)
	s.mu.Unlock()
	if status, ok := s.fileStatus[path]; ok {
		w.WriteHeader(status)
		return
	}
	data, ok := s.files[path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, _ = w.Write(data)
}

func (s *stubHost) RawFileURL(
	_ domain.Namespace,
	ref string,
	file string,
) (string, error) {
	return s.server.URL + "/raw/" + ref + "/" + file, nil
}

func (s *stubHost) BlobFileURL(
	_ domain.Namespace,
	ref string,
	file string,
) (string, error) {
	return "https://github.com/acme/tool/blob/" + ref + "/" + file, nil
}

func (s *stubHost) RepoPageURL(
	_ domain.Namespace,
) string {
	if s.page == "" && s.pageStatus == 0 {
		return ""
	}
	return s.server.URL + repoPagePath
}

func (s *stubHost) DefaultBranches() []string { return nil }

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
	if s.assetsWait != nil {
		<-s.assetsWait
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assetCall++
	return s.assets, s.assetsErr
}

func (s *stubHost) readmeReads() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	reads := make([]string, 0, len(s.rawCalls))
	for _, path := range s.rawCalls {
		if slices.Contains(media.IconProbePaths(), path) {
			continue
		}
		reads = append(reads, path)
	}
	return reads
}

func newDrafter(
	t *testing.T,
	host *stubHost,
	pk picker.Picker,
) gather.Drafter {
	t.Helper()
	host.start(t)
	return gather.New(lookupOf(host), pk, testTimeout)
}

func pageWith(
	meta ...string,
) string {
	var b strings.Builder
	b.WriteString("<!doctype html><html><head>")
	for i := 0; i+1 < len(meta); i += 2 {
		fmt.Fprintf(&b, `<meta property=%q content=%q>`, meta[i], meta[i+1])
	}
	b.WriteString("</head><body></body></html>")
	return b.String()
}

func encodePNG(
	t *testing.T,
	width int,
	height int,
) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewGray(image.Rect(0, 0, width, height))))
	return buf.Bytes()
}

type stubPicker struct {
	picks map[domain.OS]picker.Pick
	repos []string
}

func (s *stubPicker) Pick(
	repo string,
	_ []domain.ReleaseAsset,
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

func realAssets() []domain.ReleaseAsset {
	return []domain.ReleaseAsset{
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
		Asset:     domain.ReleaseAsset{Name: name, URL: "https://example.test/" + name, Digest: digestA},
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
) models.Reason {
	t.Helper()
	require.ErrorIs(t, err, models.ErrNotFletchable)
	var nf models.NotFletchableError
	require.ErrorAs(t, err, &nf)
	return nf.Reason
}

func TestDrafter_Draft_RendersParseableManifest(t *testing.T) {
	host := &stubHost{
		assets: realAssets(),
		page: pageWith(
			"og:description", "Tool searches things. Contribute to acme/tool development by creating an account on GitHub.",
			"og:image", "/img/social.png",
		),
		images: map[string][]byte{"/img/social.png": encodePNG(t, 1280, 640)},
		files: map[string][]byte{
			"README.md": []byte("# tool\n\nTool searches things fast.\n\nSee [docs](docs/guide.md).\n\n![shot](shot.png)\n"),
			"logo.svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"></svg>`),
		},
	}
	f := newDrafter(t, host, picker.New())

	draft, err := f.Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, draft.Manifest)
	assert.Equal(t, "tool", arrow.Name)
	assert.Equal(t, "Tool searches things.", arrow.Description)
	assert.Equal(t, host.server.URL+repoPagePath, arrow.URL)
	assert.Equal(t, host.server.URL+"/raw/v1.0.0/logo.svg", arrow.Media.Icon)
	assert.Equal(t, host.server.URL+"/img/social.png", arrow.Media.Banner)
	assert.Contains(t, arrow.Readme, "Tool searches things fast.")
	assert.Contains(t, arrow.Readme, "https://github.com/acme/tool/blob/v1.0.0/docs/guide.md")
	assert.Contains(t, arrow.Readme, host.server.URL+"/raw/v1.0.0/shot.png")
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

func TestDrafter_Draft_HostWithoutARepoPageHasNoDescription(t *testing.T) {
	host := &stubHost{assets: realAssets()}

	draft, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, draft.Manifest)
	assert.Empty(t, arrow.Description)
	assert.Empty(t, arrow.URL)
	assert.Empty(t, arrow.Media.Icon)
	assert.Empty(t, arrow.Media.Banner)
	assert.Zero(t, host.pageCall)
}

func TestDrafter_Draft_ReparseKeepsGenerator(t *testing.T) {
	host := &stubHost{assets: realAssets(), page: pageWith("og:description", "d")}
	f := newDrafter(t, host, picker.New())
	draft, err := f.Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	first := parse(t, draft.Manifest)
	second := parse(t, draft.Manifest)

	assert.Equal(t, first.Generator, second.Generator)
	assert.Equal(t, domain.ArrowOriginInferred, second.Origin())
	assert.NotEmpty(t, second.Generator.Confidence)
}

func TestDrafter_Draft_PassesRepoNameToPicker(t *testing.T) {
	pk := &stubPicker{picks: map[domain.OS]picker.Pick{
		domain.OSLinuxAMD64: exactPick("tool-linux.tar.gz", picker.FormatArchive),
	}}
	host := &stubHost{assets: realAssets()}

	_, err := newDrafter(t, host, pk).Draft(context.Background(), testNS, testTag)

	require.NoError(t, err)
	require.Len(t, pk.repos, len(domain.AllOS()))
	for _, repo := range pk.repos {
		assert.Equal(t, "tool", repo)
	}
}

func TestDrafter_NotFletchable_UnknownHost(t *testing.T) {
	for _, lookup := range []hosts.Lookup{hosts.None, nil} {
		f := gather.New(lookup, picker.New(), testTimeout)

		_, err := f.Draft(context.Background(), testNS, testTag)

		assert.Equal(t, models.ReasonHostUnsupported, notFletchableReason(t, err))
	}
}

func TestDrafter_NotFletchable(t *testing.T) {
	testCases := []struct {
		name  string
		host  *stubHost
		picks map[domain.OS]picker.Pick
		want  models.Reason
	}{
		{
			name: "release without assets",
			host: &stubHost{},
			want: models.ReasonNoReleaseAssets,
		},
		{
			name: "only unusable assets picked",
			host: &stubHost{assets: realAssets()},
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: {Asset: domain.ReleaseAsset{Name: "tool.tar.gz", URL: "https://example.test/tool.tar.gz"}},
				domain.OSLinuxARM64: {Asset: domain.ReleaseAsset{Name: "x", URL: "https://example.test/..", Digest: digestA}},
			},
			want: models.ReasonNoUsableAsset,
		},
		{
			name:  "no asset picked",
			host:  &stubHost{assets: realAssets()},
			picks: map[domain.OS]picker.Pick{},
			want:  models.ReasonNoUsableAsset,
		},
		{
			name:  "nothing picked even with digests",
			host:  &stubHost{assets: digestless(realAssets())},
			picks: map[domain.OS]picker.Pick{},
			want:  models.ReasonNoUsableAsset,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDrafter(t, tc.host, &stubPicker{picks: tc.picks})

			_, fletchErr := f.Draft(context.Background(), testNS, testTag)

			assert.Equal(t, tc.want, notFletchableReason(t, fletchErr))
		})
	}
}

func TestDrafter_NotFletchable_NoDigest(t *testing.T) {
	testCases := []struct {
		name   string
		assets []domain.ReleaseAsset
		want   models.Reason
	}{
		{name: "every asset lacks a digest", assets: digestless(realAssets()), want: models.ReasonNoDigest},
		{name: "one asset lacks a digest but another is usable", assets: withDigestOn(digestless(realAssets()), 0), want: ""},
		{
			name:   "digestless assets that would not be picked either",
			assets: []domain.ReleaseAsset{{Name: "tool-1.0.0.deb", URL: "https://example.test/tool.deb"}},
			want:   models.ReasonNoUsableAsset,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDrafter(t, &stubHost{assets: tc.assets}, picker.New())

			_, err := f.Draft(context.Background(), testNS, testTag)

			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			assert.Equal(t, tc.want, notFletchableReason(t, err))
		})
	}
}

func digestless(
	assets []domain.ReleaseAsset,
) []domain.ReleaseAsset {
	out := make([]domain.ReleaseAsset, len(assets))
	for i, a := range assets {
		a.Digest = ""
		out[i] = a
	}
	return out
}

func withDigestOn(
	assets []domain.ReleaseAsset,
	index int,
) []domain.ReleaseAsset {
	assets[index].Digest = digestA
	return assets
}

func TestDrafter_Draft_HostErrorsPropagate(t *testing.T) {
	cjk := []byte("这是一个非常快速的搜索工具,支持正则表达式")
	testCases := []struct {
		name string
		host *stubHost
		want error
	}{
		{
			name: "release assets",
			host: &stubHost{assetsErr: errBoom},
			want: errBoom,
		},
		{
			name: "repo page",
			host: &stubHost{assets: realAssets(), pageStatus: http.StatusInternalServerError},
			want: resolver.ErrFetchFailed,
		},
		{
			name: "default readme",
			host: &stubHost{assets: realAssets(), fileStatus: map[string]int{"readme.md": http.StatusInternalServerError}},
			want: resolver.ErrFetchFailed,
		},
		{
			name: "english readme",
			host: &stubHost{
				assets:     realAssets(),
				files:      map[string][]byte{"README.md": cjk},
				fileStatus: map[string]int{"README_EN.md": http.StatusInternalServerError},
			},
			want: resolver.ErrFetchFailed,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDrafter(t, tc.host, picker.New())

			_, err := f.Draft(context.Background(), testNS, testTag)

			require.ErrorIs(t, err, tc.want)
			assert.NotErrorIs(t, err, models.ErrNotFletchable)
		})
	}
}

func TestDrafter_Draft_UnnamableReadmeFails(t *testing.T) {
	host := &unnamableHost{stubHost: &stubHost{assets: realAssets()}}
	host.start(t)

	_, err := gather.New(lookupOf(host), picker.New(), testTimeout).Draft(context.Background(), testNS, testTag)

	require.ErrorIs(t, err, errBoom)
}

type unnamableHost struct {
	*stubHost
}

func (u *unnamableHost) RawFileURL(
	_ domain.Namespace,
	_ string,
	file string,
) (string, error) {
	if file == "README.md" {
		return "", errBoom
	}
	return u.server.URL + "/missing", nil
}

func TestDrafter_Draft_DropsUnusablePicks(t *testing.T) {
	pk := &stubPicker{picks: map[domain.OS]picker.Pick{
		domain.OSLinuxAMD64: exactPick("tool-linux.tar.gz", picker.FormatArchive),
		domain.OSLinuxARM64: withMatch(
			picker.Pick{Asset: domain.ReleaseAsset{Name: "x", URL: "https://example.test/%24%7BHOME%7D", Digest: digestA}},
			picker.MatchAssumed,
			false,
		),
	}}

	draft, err := newDrafter(t, &stubHost{assets: realAssets()}, pk).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, draft.Manifest)
	assert.Contains(t, arrow.Targets, domain.OSLinuxAMD64)
	assert.NotContains(t, arrow.Targets, domain.OSLinuxARM64)
	assert.Equal(t, confidence.ConfidenceHigh, draft.Report.Confidence)
	assert.Empty(t, draft.Report.Warnings)
}

func TestDrafter_Draft_URLComesFromRepoPageURL(t *testing.T) {
	host := &stubHost{assets: realAssets(), page: pageWith("og:description", "d")}

	draft, err := newDrafter(t, host, picker.New()).Draft(
		context.Background(),
		domain.Namespace("gitlab.example.com/acme/tool@v1.0.0"),
		testTag,
	)
	require.NoError(t, err)

	assert.Equal(t, host.server.URL+repoPagePath, parse(t, draft.Manifest).URL)
}

func withMatch(
	pick picker.Pick,
	match picker.Match,
	nameMatch bool,
) picker.Pick {
	pick.Match = match
	pick.NameMatch = nameMatch
	return pick
}

func TestDrafter_LowConfidenceIsNotFletchable(t *testing.T) {
	testCases := []struct {
		name  string
		picks map[domain.OS]picker.Pick
	}{
		{
			name: "name mismatch",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64: withMatch(exactPick("other-linux.tar.gz", picker.FormatArchive), picker.MatchExact, false),
			},
		},
		{
			name: "one mismatched pick among medium ones",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:  withMatch(exactPick("other.tar.gz", picker.FormatArchive), picker.MatchAssumed, false),
				domain.OSDarwinARM64: withMatch(exactPick("tool-mac.zip", picker.FormatArchive), picker.MatchEmulated, true),
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDrafter(t, &stubHost{assets: realAssets()}, &stubPicker{picks: tc.picks})

			draft, err := f.Draft(context.Background(), testNS, testTag)

			var nf models.NotFletchableError
			require.ErrorAs(t, err, &nf)
			assert.Equal(t, models.ReasonLowConfidence, nf.Reason)
			assert.Empty(t, draft.Manifest)
		})
	}
}

func TestDrafter_Draft_MediumConfidenceWarningsReachTheGenerator(t *testing.T) {
	pk := &stubPicker{picks: map[domain.OS]picker.Pick{
		domain.OSLinuxAMD64:  withMatch(exactPick("tool-linux.tar.gz", picker.FormatArchive), picker.MatchAssumed, true),
		domain.OSDarwinARM64: withMatch(exactPick("tool-mac.zip", picker.FormatArchive), picker.MatchEmulated, true),
	}}

	draft, err := newDrafter(t, &stubHost{assets: realAssets()}, pk).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, draft.Manifest)
	wantWarnings := []string{confidence.WarningAssumedArch, confidence.WarningEmulated}
	assert.Equal(t, confidence.ConfidenceMedium, draft.Report.Confidence)
	assert.Equal(t, wantWarnings, draft.Report.Warnings)
	assert.Equal(t, string(confidence.ConfidenceMedium), arrow.Generator.Confidence)
	assert.Equal(t, wantWarnings, arrow.Generator.Warnings)
}
