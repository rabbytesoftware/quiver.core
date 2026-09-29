package media

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	githubRawTemplate  = "https://raw.githubusercontent.com/owner/repo/{branch}/{file}"
	githubBlobTemplate = "https://github.com/owner/repo/blob/{branch}/{file}"
	testRef            = "v1.0.0"
)

var errMissing = errors.New("missing")

type stubHost struct {
	files        map[string][]byte
	images       map[string][]byte
	rawTemplate  string
	blobTemplate string
	noTemplates  bool
}

func (s *stubHost) templates() (string, string) {
	if s.rawTemplate == "" {
		return githubRawTemplate, githubBlobTemplate
	}
	return s.rawTemplate, s.blobTemplate
}

func (s *stubHost) RawFileURL(
	_ domain.Namespace,
	ref string,
	file string,
) (string, error) {
	raw, _ := s.templates()
	return fill(raw, ref, file, s.noTemplates)
}

func (s *stubHost) BlobFileURL(
	_ domain.Namespace,
	ref string,
	file string,
) (string, error) {
	_, blob := s.templates()
	return fill(blob, ref, file, s.noTemplates)
}

func fill(
	template string,
	ref string,
	file string,
	missing bool,
) (string, error) {
	if missing {
		return "", errMissing
	}
	return strings.NewReplacer("{branch}", ref, "{file}", file).Replace(template), nil
}

func (s *stubHost) RepoPageURL(
	_ domain.Namespace,
) string {
	return ""
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
	return nil, nil
}

func (s *stubHost) fetch(
	_ context.Context,
	url string,
) ([]byte, error) {
	if data, ok := s.images[url]; ok {
		return data, nil
	}
	raw, _ := s.templates()
	path, ok := strings.CutPrefix(url, strings.NewReplacer("{branch}", testRef, "{file}", "").Replace(raw))
	if !ok {
		return nil, errMissing
	}
	data, ok := s.files[path]
	if !ok {
		return nil, errMissing
	}
	return data, nil
}

func resolveAll(
	ctx context.Context,
	host *stubHost,
	ns domain.Namespace,
	socialImage string,
	readmeRaw []byte,
) domain.ArrowMedia {
	probed := ProbeIcon(ctx, host.fetch, host, ns, testRef)
	return Resolve(ctx, host.fetch, host, ns, testRef, socialImage, readmeRaw, probed)
}

func githubURLs() fileURLs {
	return fileURLsOf(&stubHost{}, testNS, testRef)
}

const (
	testNS          = domain.Namespace("github.com/owner/repo@v1.0.0")
	githubRawPrefix = "https://raw.githubusercontent.com/owner/repo/v1.0.0/"
)

func TestResolve_IconProbePathHitInOrder(
	t *testing.T,
) {
	host := &stubHost{files: map[string][]byte{
		"src-tauri/icons/icon.png": encodePNG(t, 256, 256),
	}}

	media := resolveAll(context.Background(), host, testNS, "", nil)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/src-tauri/icons/icon.png", media.Icon)
}

func TestResolve_IconProbeSVGAcceptedRegardlessOfSize(
	t *testing.T,
) {
	host := &stubHost{files: map[string][]byte{
		"logo.svg": []byte(`<svg width="8" height="8"><path/></svg>`),
	}}

	media := resolveAll(context.Background(), host, testNS, "", nil)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/logo.svg", media.Icon)
}

func TestResolve_IconProbeSkipsTooSmallPNGFallsToNextCandidate(
	t *testing.T,
) {
	host := &stubHost{files: map[string][]byte{
		"src-tauri/icons/icon.png": encodePNG(t, 64, 64),
		"build/icon.png":           encodePNG(t, 256, 256),
	}}

	media := resolveAll(context.Background(), host, testNS, "", nil)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/build/icon.png", media.Icon)
}

func TestResolve_IconFallsBackToReadmeSquareImage(
	t *testing.T,
) {
	readmeRaw := []byte("![Icon](icon-art.png)\n")
	host := &stubHost{files: map[string][]byte{
		"icon-art.png": encodePNG(t, 300, 300),
	}}

	media := resolveAll(context.Background(), host, testNS, "", readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/icon-art.png", media.Icon)
}

func TestResolve_IconSkipsBadgeImagesInReadme(
	t *testing.T,
) {
	readmeRaw := []byte("![Build](https://img.shields.io/x.svg)\n![Icon](icon-art.png)\n")
	host := &stubHost{files: map[string][]byte{
		"icon-art.png": encodePNG(t, 300, 300),
	}}

	media := resolveAll(context.Background(), host, testNS, "", readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/icon-art.png", media.Icon)
}

func TestResolve_IconIgnoresReadmeImagePastLineLimit(
	t *testing.T,
) {
	lines := ""
	for i := 0; i < 61; i++ {
		lines += "prose line\n"
	}
	lines += "![Icon](icon-art.png)\n"
	host := &stubHost{files: map[string][]byte{
		"icon-art.png": encodePNG(t, 300, 300),
	}}

	media := resolveAll(context.Background(), host, testNS, "", []byte(lines))

	assert.Empty(t, media.Icon)
}

func TestResolve_BannerSocialImage(t *testing.T) {
	const social = "https://images.example.test/social.png"
	testCases := []struct {
		name       string
		image      []byte
		readme     string
		wantBanner string
	}{
		{name: "banner shaped", image: encodePNG(t, 1280, 640), wantBanner: social},
		{name: "square falls back to the readme", image: encodePNG(t, 300, 300), readme: "![s](shot.png)\n", wantBanner: githubRawPrefix + "shot.png"},
		{name: "unsniffable", image: []byte("not an image")},
		{name: "unreachable"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host := &stubHost{files: map[string][]byte{"shot.png": encodePNG(t, 1200, 400)}}
			if tc.image != nil {
				host.images = map[string][]byte{social: tc.image}
			}

			media := resolveAll(context.Background(), host, testNS, social, []byte(tc.readme))

			assert.Equal(t, tc.wantBanner, media.Banner)
			assert.Empty(t, media.Icon)
		})
	}
}

func TestResolve_BannerFallsBackToWideReadmeImage(
	t *testing.T,
) {
	readmeRaw := []byte("![Screenshot](shot.png)\n")
	host := &stubHost{files: map[string][]byte{
		"shot.png": encodePNG(t, 1200, 400),
	}}

	media := resolveAll(context.Background(), host, testNS, "", readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/shot.png", media.Banner)
}

func TestResolve_BannerFallsBackToScreenshotShapedImage(
	t *testing.T,
) {
	readmeRaw := []byte("![Screenshot](shot.png)\n")
	host := &stubHost{files: map[string][]byte{
		"shot.png": encodePNG(t, 640, 360),
	}}

	media := resolveAll(context.Background(), host, testNS, "", readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/shot.png", media.Banner)
}

func TestResolve_BannerNoneWhenNothingQualifies(
	t *testing.T,
) {
	readmeRaw := []byte("![Screenshot](shot.png)\n")
	host := &stubHost{files: map[string][]byte{
		"shot.png": encodePNG(t, 100, 100),
	}}

	media := resolveAll(context.Background(), host, testNS, "", readmeRaw)

	assert.Empty(t, media.Banner)
}

func TestResolve_BannerSkipsBadgeImages(
	t *testing.T,
) {
	readmeRaw := []byte("![Build](https://img.shields.io/x.svg)\n![Screenshot](shot.png)\n")
	host := &stubHost{files: map[string][]byte{
		"shot.png": encodePNG(t, 1200, 400),
	}}

	media := resolveAll(context.Background(), host, testNS, "", readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/shot.png", media.Banner)
}

func TestResolve_SkipsUnresolvableExternalImage(
	t *testing.T,
) {
	readmeRaw := []byte("![Screenshot](https://someone-elses-cdn.example/shot.png)\n")
	host := &stubHost{files: map[string][]byte{}}

	media := resolveAll(context.Background(), host, testNS, "", readmeRaw)

	assert.Empty(t, media.Icon)
	assert.Empty(t, media.Banner)
}

func TestResolve_SkipsCandidateWhenFileMissing(
	t *testing.T,
) {
	readmeRaw := []byte("![Icon](missing.png)\n![Icon](icon-art.png)\n")
	host := &stubHost{files: map[string][]byte{
		"icon-art.png": encodePNG(t, 300, 300),
	}}

	media := resolveAll(context.Background(), host, testNS, "", readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/icon-art.png", media.Icon)
}

func TestResolve_SkipsCandidateWhenDataUnsniffable(
	t *testing.T,
) {
	readmeRaw := []byte("![Icon](garbage.png)\n![Icon](icon-art.png)\n")
	host := &stubHost{files: map[string][]byte{
		"garbage.png":  []byte("not an image"),
		"icon-art.png": encodePNG(t, 300, 300),
	}}

	media := resolveAll(context.Background(), host, testNS, "", readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/icon-art.png", media.Icon)
}

func TestResolvePath_ProtocolRelativeRejected(
	t *testing.T,
) {
	_, ok := githubURLs().resolve("//cdn.example.com/img.png")

	assert.False(t, ok)
}

func TestResolvePath_RelativeIsCleaned(
	t *testing.T,
) {
	path, ok := githubURLs().resolve("./doc/logo.png")

	require.True(t, ok)
	assert.Equal(t, "doc/logo.png", path)
}

func TestResolvePath_SameRepoRawURLResolves(
	t *testing.T,
) {
	path, ok := githubURLs().resolve("https://raw.githubusercontent.com/owner/repo/main/doc/logo.png")

	require.True(t, ok)
	assert.Equal(t, "doc/logo.png", path)
}

func TestResolvePath_SameRepoBlobURLResolves(
	t *testing.T,
) {
	path, ok := githubURLs().resolve("https://github.com/owner/repo/blob/main/doc/logo.png")

	require.True(t, ok)
	assert.Equal(t, "doc/logo.png", path)
}

func TestResolvePath_MalformedSameRepoRawURLRejected(
	t *testing.T,
) {
	_, ok := githubURLs().resolve("https://raw.githubusercontent.com/owner/repo/main")

	assert.False(t, ok)
}

func TestResolvePath_OtherRepoURLRejected(
	t *testing.T,
) {
	_, ok := githubURLs().resolve("https://raw.githubusercontent.com/other/repo/main/x.png")

	assert.False(t, ok)
}

func TestFetchProbe_MissingFileFails(
	t *testing.T,
) {
	host := &stubHost{files: map[string][]byte{}}

	data, ok := fetchProbe(context.Background(), host.fetch, githubRawPrefix+"missing.png")

	assert.False(t, ok)
	assert.Nil(t, data)
}

func TestIsBannerShaped_Classification(
	t *testing.T,
) {
	testCases := []struct {
		name string
		dim  Dimensions
		want bool
	}{
		{name: "exactly 2 to 1 wide", dim: Dimensions{Width: 400, Height: 200}, want: true},
		{name: "very wide", dim: Dimensions{Width: 1200, Height: 200}, want: true},
		{name: "screenshot shaped 16 by 9", dim: Dimensions{Width: 640, Height: 360}, want: true},
		{name: "square rejected", dim: Dimensions{Width: 300, Height: 300}, want: false},
		{name: "small landscape rejected", dim: Dimensions{Width: 300, Height: 250}, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isBannerShaped(tc.dim))
		})
	}
}
