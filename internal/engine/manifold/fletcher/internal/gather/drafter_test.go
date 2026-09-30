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
	hosts.Host
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
	meta       domain.RepoMetadata
	metaErr    error
	metaCall   int
	avatarPath string
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

func (s *stubHost) OwnerAvatarURL(
	_ domain.Namespace,
) string {
	if s.avatarPath == "" {
		return ""
	}
	return s.server.URL + s.avatarPath
}

func (s *stubHost) RepoPageURL(
	_ domain.Namespace,
) string {
	if s.page == "" && s.pageStatus == 0 {
		return ""
	}
	return s.server.URL + repoPagePath
}

func (s *stubHost) DefaultBranches() []string {
	return []string{"main"}
}

func (s *stubHost) RepoMetadata(
	_ context.Context,
	_ domain.Namespace,
) (domain.RepoMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metaCall++
	return s.meta, s.metaErr
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
		if isIconProbe(path) {
			continue
		}
		reads = append(reads, path)
	}
	return reads
}

func isIconProbe(
	path string,
) bool {
	if strings.HasPrefix(path, "src-tauri/icons/") {
		return true
	}
	base := path[strings.LastIndex(path, "/")+1:]
	for _, name := range []string{"icon.", "logo.", "app-icon."} {
		if strings.HasPrefix(base, name) {
			return true
		}
	}
	return false
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

	manifest, err := f.Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, manifest)
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
	assert.NotEmpty(t, arrow.Generator.Confidence)
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

func TestDrafter_Draft_RepoMetadataFillsDescriptionAndAvatarIcon(t *testing.T) {
	host := &stubHost{
		assets: realAssets(),
		page:   pageWith("og:description", "Scraped description."),
		meta:   domain.RepoMetadata{Description: "Authored description.", AvatarURL: "https://avatars.example.test/u/9?v=4"},
	}

	manifest, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, manifest)
	assert.Equal(t, "Authored description.", arrow.Description)
	assert.Equal(t, "https://avatars.example.test/u/9?v=4", arrow.Media.Icon)
	assert.Equal(t, 1, host.metaCall)
}

func TestDrafter_Draft_RepoIconBeatsTheAvatar(t *testing.T) {
	host := &stubHost{
		assets: realAssets(),
		meta:   domain.RepoMetadata{AvatarURL: "https://avatars.example.test/u/9?v=4"},
		files:  map[string][]byte{"assets/logo.png": encodePNG(t, 256, 256)},
	}

	manifest, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	assert.Equal(t, host.server.URL+"/raw/v1.0.0/assets/logo.png", parse(t, manifest).Media.Icon)
}

func TestDrafter_Draft_UnmeteredAvatarIsTheIconWhenMetadataFails(t *testing.T) {
	host := &stubHost{
		assets:     realAssets(),
		metaErr:    errors.New("rate limited"),
		avatarPath: "/acme.png",
		images:     map[string][]byte{"/acme.png": encodePNG(t, 460, 460)},
	}

	manifest, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	assert.Equal(t, host.server.URL+"/acme.png", parse(t, manifest).Media.Icon)
}

func TestDrafter_Draft_UnmeteredAvatarBeatsTheMeteredOne(t *testing.T) {
	host := &stubHost{
		assets:     realAssets(),
		meta:       domain.RepoMetadata{AvatarURL: "https://avatars.example.test/u/9?v=4"},
		avatarPath: "/acme.png",
		images:     map[string][]byte{"/acme.png": encodePNG(t, 460, 460)},
	}

	manifest, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	assert.Equal(t, host.server.URL+"/acme.png", parse(t, manifest).Media.Icon)
}

func TestDrafter_Draft_RepoIconBeatsTheUnmeteredAvatar(t *testing.T) {
	host := &stubHost{
		assets:     realAssets(),
		avatarPath: "/acme.png",
		images:     map[string][]byte{"/acme.png": encodePNG(t, 460, 460)},
		files:      map[string][]byte{"assets/logo.png": encodePNG(t, 256, 256)},
	}

	manifest, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	assert.Equal(t, host.server.URL+"/raw/v1.0.0/assets/logo.png", parse(t, manifest).Media.Icon)
}

func TestDrafter_Draft_UnmeteredAvatarThatIsNotAnImageFallsBackToTheMeteredOne(t *testing.T) {
	host := &stubHost{
		assets:     realAssets(),
		meta:       domain.RepoMetadata{AvatarURL: "https://avatars.example.test/u/9?v=4"},
		avatarPath: "/acme.png",
		images:     map[string][]byte{"/acme.png": []byte("<html>sign in</html>")},
	}

	manifest, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	assert.Equal(t, "https://avatars.example.test/u/9?v=4", parse(t, manifest).Media.Icon)
}

func TestDrafter_Draft_RepoMetadataFailureDegradesToThePage(t *testing.T) {
	host := &stubHost{
		assets:  realAssets(),
		page:    pageWith("og:description", "Scraped description."),
		metaErr: errors.New("rate limited"),
	}

	manifest, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, manifest)
	assert.Equal(t, "Scraped description.", arrow.Description)
	assert.Empty(t, arrow.Media.Icon)
}

func TestDrafter_Draft_HostWithoutARepoPageHasNoDescription(t *testing.T) {
	host := &stubHost{assets: realAssets()}

	manifest, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, manifest)
	assert.Empty(t, arrow.Description)
	assert.Empty(t, arrow.URL)
	assert.Empty(t, arrow.Media.Icon)
	assert.Empty(t, arrow.Media.Banner)
	assert.Zero(t, host.pageCall)
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
	f := gather.New(hosts.None, picker.New(), testTimeout)

	_, err := f.Draft(context.Background(), testNS, testTag)

	assert.Equal(t, models.ReasonHostUnsupported, notFletchableReason(t, err))
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

	manifest, err := newDrafter(t, &stubHost{assets: realAssets()}, pk).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, manifest)
	assert.Contains(t, arrow.Targets, domain.OSLinuxAMD64)
	assert.NotContains(t, arrow.Targets, domain.OSLinuxARM64)
	assert.Equal(t, string(confidence.ConfidenceHigh), arrow.Generator.Confidence)
	assert.Empty(t, arrow.Generator.Warnings)
}

func TestDrafter_Draft_URLComesFromRepoPageURL(t *testing.T) {
	host := &stubHost{assets: realAssets(), page: pageWith("og:description", "d")}

	manifest, err := newDrafter(t, host, picker.New()).Draft(
		context.Background(),
		domain.Namespace("gitlab.example.com/acme/tool@v1.0.0"),
		testTag,
	)
	require.NoError(t, err)

	assert.Equal(t, host.server.URL+repoPagePath, parse(t, manifest).URL)
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
			name: "every platform mismatched",
			picks: map[domain.OS]picker.Pick{
				domain.OSLinuxAMD64:  withMatch(exactPick("other.tar.gz", picker.FormatArchive), picker.MatchAssumed, false),
				domain.OSDarwinARM64: withMatch(exactPick("tool-mac.zip", picker.FormatArchive), picker.MatchEmulated, false),
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDrafter(t, &stubHost{assets: realAssets()}, &stubPicker{picks: tc.picks})

			manifest, err := f.Draft(context.Background(), testNS, testTag)

			var nf models.NotFletchableError
			require.ErrorAs(t, err, &nf)
			assert.Equal(t, models.ReasonLowConfidence, nf.Reason)
			assert.Empty(t, manifest)
		})
	}
}

func TestDrafter_Draft_MediumConfidenceWarningsReachTheGenerator(t *testing.T) {
	pk := &stubPicker{picks: map[domain.OS]picker.Pick{
		domain.OSLinuxAMD64:  withMatch(exactPick("tool-linux.tar.gz", picker.FormatArchive), picker.MatchAssumed, true),
		domain.OSDarwinARM64: withMatch(exactPick("tool-mac.zip", picker.FormatArchive), picker.MatchEmulated, true),
	}}

	manifest, err := newDrafter(t, &stubHost{assets: realAssets()}, pk).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, manifest)
	wantWarnings := []string{confidence.WarningAssumedArch, confidence.WarningEmulated}
	assert.Equal(t, string(confidence.ConfidenceMedium), arrow.Generator.Confidence)
	assert.Equal(t, wantWarnings, arrow.Generator.Warnings)
}

func TestDrafter_Draft_AnInconsistentPlatformIsDroppedNotTheWholeDraft(t *testing.T) {
	pk := &stubPicker{picks: map[domain.OS]picker.Pick{
		domain.OSLinuxAMD64:   exactPick("tool-linux.tar.gz", picker.FormatArchive),
		domain.OSWindowsAMD64: withMatch(exactPick("other.zip", picker.FormatArchive), picker.MatchExact, false),
	}}

	manifest, err := newDrafter(t, &stubHost{assets: realAssets()}, pk).Draft(context.Background(), testNS, testTag)
	require.NoError(t, err)

	arrow := parse(t, manifest)
	assert.Contains(t, arrow.Targets, domain.OSLinuxAMD64)
	assert.NotContains(t, arrow.Targets, domain.OSWindowsAMD64)
	assert.Contains(t, arrow.Generator.Warnings, confidence.WarningNameMismatch)
}

func releaseOf(
	names ...string,
) []domain.ReleaseAsset {
	out := make([]domain.ReleaseAsset, 0, len(names))
	for _, n := range names {
		out = append(out, domain.ReleaseAsset{Name: n, URL: "https://example.test/" + n, Digest: "sha256:" + n})
	}
	return out
}

func TestDrafter_Draft_RealPickerReleases(t *testing.T) {
	testCases := []struct {
		name        string
		ns          domain.Namespace
		assets      []domain.ReleaseAsset
		wantTargets []domain.OS
		absent      []domain.OS
	}{
		{
			name: "product differs from the repo name",
			ns:   "github.com/zen-browser/desktop@v1.0.0",
			assets: releaseOf(
				"zen-x86_64.AppImage",
				"zen.linux-x86_64.tar.xz",
				"zen.macos-universal.dmg",
			),
			wantTargets: []domain.OS{domain.OSLinuxAMD64, domain.OSDarwinARM64},
		},
		{
			name: "a stray windows product is dropped while the rest ships",
			ns:   "github.com/pingdotgg/t3code@v1.0.0",
			assets: releaseOf(
				"T3-Code-0.0.44-x86_64.AppImage",
				"T3-Code-0.0.44-arm64.dmg",
				"t3-0.0.44-win32-x64.zip",
			),
			wantTargets: []domain.OS{domain.OSLinuxAMD64, domain.OSDarwinARM64},
			absent:      []domain.OS{domain.OSWindowsAMD64},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			manifest, err := newDrafter(t, &stubHost{assets: tc.assets}, picker.New()).Draft(context.Background(), tc.ns, testTag)
			require.NoError(t, err)

			arrow := parse(t, manifest)
			for _, platform := range tc.wantTargets {
				assert.Contains(t, arrow.Targets, platform)
			}
			for _, platform := range tc.absent {
				assert.NotContains(t, arrow.Targets, platform)
			}
			assert.NotEqual(t, string(confidence.ConfidenceLow), arrow.Generator.Confidence)
		})
	}
}

func TestDrafter_Draft_MonorepoWithUnrelatedProductsStaysRefused(t *testing.T) {
	assets := releaseOf("alpha-linux-x86_64.tar.gz", "beta-linux-x86_64.tar.gz")

	_, err := newDrafter(t, &stubHost{assets: assets}, picker.New()).Draft(context.Background(), testNS, testTag)

	require.Error(t, err)
	assert.Equal(t, models.ReasonNoUsableAsset, notFletchableReason(t, err))
}

func TestDrafter_Draft_WindowsInstallerOnlyIsNotFletchable(t *testing.T) {
	assets := releaseOf("Tool-1.0.0-x64-setup.exe", "Tool.installer.exe")

	_, err := newDrafter(t, &stubHost{assets: assets}, picker.New()).Draft(context.Background(), testNS, testTag)

	require.Error(t, err)
	assert.Equal(t, models.ReasonNoUsableAsset, notFletchableReason(t, err))
}

func TestDrafter_Draft_RollingTag(t *testing.T) {
	testCases := []struct {
		name         string
		tag          string
		wantChecksum bool
		wantWarning  bool
	}{
		{name: "versioned release pins the checksum", tag: "v1.2.3", wantChecksum: true, wantWarning: false},
		{name: "pointer tag is unpinned and warned", tag: "tip", wantChecksum: false, wantWarning: true},
		{name: "nightly pointer tag is unpinned and warned", tag: "nightly", wantChecksum: false, wantWarning: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host := &stubHost{assets: releaseOf("tool-linux-x86_64.tar.gz")}

			manifest, err := newDrafter(t, host, picker.New()).Draft(context.Background(), testNS, tc.tag)
			require.NoError(t, err)

			arrow := parse(t, manifest)
			target := arrow.Targets[domain.OSLinuxAMD64]
			require.NotEmpty(t, target.Lifecycle.Install)
			assert.Equal(t, tc.wantChecksum, strings.Contains(string(manifest), "checksum:"))
			assert.Equal(t, tc.wantWarning, slices.Contains(arrow.Generator.Warnings, confidence.WarningUnpinnedRollingTag))
			if tc.wantWarning {
				assert.Equal(t, string(confidence.ConfidenceMedium), arrow.Generator.Confidence)
			}
		})
	}
}

func TestDrafter_Draft_RollingTagAcceptsAssetsWithoutADigest(t *testing.T) {
	assets := []domain.ReleaseAsset{
		{Name: "tool-linux-x86_64.tar.gz", URL: "https://example.test/a.tar.gz"},
		{Name: "tool-darwin-arm64.tar.gz", URL: "https://example.test/b.tar.gz", Digest: digestA},
	}

	manifest, err := newDrafter(t, &stubHost{assets: assets}, picker.New()).Draft(context.Background(), testNS, "tip")
	require.NoError(t, err)

	arrow := parse(t, manifest)
	assert.Contains(t, arrow.Targets, domain.OSLinuxAMD64)
	assert.Contains(t, arrow.Targets, domain.OSDarwinARM64)
	assert.NotContains(t, string(manifest), "checksum:")
}

func TestDrafter_Draft_VersionedReleaseStillRequiresADigest(t *testing.T) {
	assets := []domain.ReleaseAsset{{Name: "tool-linux-x86_64.tar.gz", URL: "https://example.test/a.tar.gz"}}

	_, err := newDrafter(t, &stubHost{assets: assets}, picker.New()).Draft(context.Background(), testNS, testTag)

	require.Error(t, err)
	assert.Equal(t, models.ReasonNoDigest, notFletchableReason(t, err))
}

func TestDrafter_Draft_LabelledAssetsResolveTheOSFromTheLabelAndTheFormatFromTheFileName(t *testing.T) {
	release := func(name, label string) domain.ReleaseAsset {
		return domain.ReleaseAsset{Name: name, Label: label, URL: "https://example.com/" + name, Digest: digestA}
	}
	host := &stubHost{assets: []domain.ReleaseAsset{
		release("GitHub.Desktop-3.6.6-checksums.txt", "GitHub Desktop 3.6.6 checksums"),
		release("GitHub.Desktop-arm64.zip", "GitHub Desktop 3.6.6 macOS arm64"),
		release("GitHub.Desktop-x64.zip", "GitHub Desktop 3.6.6 macOS x64"),
		release("GitHubDesktopSetup-x64.msi", "GitHub Desktop 3.6.6 Windows x64 MSI Installer"),
	}}
	f := newDrafter(t, host, picker.New())

	manifest, err := f.Draft(context.Background(), domain.Namespace("github.com/desktop/desktop@release-3.6.6"), "release-3.6.6")
	require.NoError(t, err)

	arrow := parse(t, manifest)
	for os, file := range map[domain.OS]string{domain.OSDarwinARM64: "GitHub.Desktop-arm64.zip", domain.OSDarwinAMD64: "GitHub.Desktop-x64.zip"} {
		target, ok := arrow.Targets[os]
		require.True(t, ok, os)
		assert.Equal(t, "Download "+file, target.Lifecycle.Install[0].Title(), os)
		want := []domain.ExposeEntry{{Name: "desktop", Path: domain.ExposeAuto}}
		assert.Equal(t, want, target.Expose.CLI, os)
		assert.Equal(t, want, target.Expose.Desktop, os)
	}
}
