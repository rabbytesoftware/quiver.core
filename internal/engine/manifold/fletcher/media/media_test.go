package media

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type stubForge struct {
	files map[string][]byte
}

func (s *stubForge) ReleaseAssets(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) ([]hosts.Asset, error) {
	return nil, nil
}

func (s *stubForge) RepoPage(
	_ context.Context,
	_ domain.Namespace,
) (hosts.RepoPage, error) {
	return hosts.RepoPage{}, nil
}

func (s *stubForge) RawFile(
	_ context.Context,
	_ domain.Namespace,
	_ string,
	path string,
) ([]byte, error) {
	data, ok := s.files[path]
	if !ok {
		return nil, hosts.ErrRawNotFound
	}
	return data, nil
}

const testNS = domain.Namespace("github.com/owner/repo@v1.0.0")

func TestResolve_IconProbePathHitInOrder(
	t *testing.T,
) {
	forge := &stubForge{files: map[string][]byte{
		"src-tauri/icons/icon.png": encodePNG(t, 256, 256),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, nil)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/src-tauri/icons/icon.png", media.Icon)
}

func TestResolve_IconProbeSVGAcceptedRegardlessOfSize(
	t *testing.T,
) {
	forge := &stubForge{files: map[string][]byte{
		"assets/logo.svg": []byte(`<svg width="8" height="8"><path/></svg>`),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, nil)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/assets/logo.svg", media.Icon)
}

func TestResolve_IconProbeSkipsTooSmallPNGFallsToNextCandidate(
	t *testing.T,
) {
	forge := &stubForge{files: map[string][]byte{
		"icon.png": encodePNG(t, 64, 64),
		"logo.png": encodePNG(t, 256, 256),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, nil)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/logo.png", media.Icon)
}

func TestResolve_IconFallsBackToReadmeSquareImage(
	t *testing.T,
) {
	readmeRaw := []byte("![Icon](icon-art.png)\n")
	forge := &stubForge{files: map[string][]byte{
		"icon-art.png": encodePNG(t, 300, 300),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/icon-art.png", media.Icon)
}

func TestResolve_IconSkipsBadgeImagesInReadme(
	t *testing.T,
) {
	readmeRaw := []byte("![Build](https://img.shields.io/x.svg)\n![Icon](icon-art.png)\n")
	forge := &stubForge{files: map[string][]byte{
		"icon-art.png": encodePNG(t, 300, 300),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, readmeRaw)

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
	forge := &stubForge{files: map[string][]byte{
		"icon-art.png": encodePNG(t, 300, 300),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, []byte(lines))

	assert.Empty(t, media.Icon)
}

func TestResolve_IconFallsBackToOrgAvatar(
	t *testing.T,
) {
	forge := &stubForge{files: map[string][]byte{}}
	page := hosts.RepoPage{OwnerIsOrg: true, OwnerAvatar: "https://github.com/owner.png"}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", page, nil)

	assert.Equal(t, "https://github.com/owner.png", media.Icon)
}

func TestResolve_IconEmptyWhenOwnerIsUser(
	t *testing.T,
) {
	forge := &stubForge{files: map[string][]byte{}}
	page := hosts.RepoPage{OwnerIsOrg: false, OwnerAvatar: "https://github.com/owner.png"}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", page, nil)

	assert.Empty(t, media.Icon)
}

func TestResolve_BannerPrefersCustomOGImage(
	t *testing.T,
) {
	forge := &stubForge{files: map[string][]byte{}}
	page := hosts.RepoPage{CustomSocialImage: true, SocialImage: "https://example.com/custom-og.png"}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", page, nil)

	assert.Equal(t, "https://example.com/custom-og.png", media.Banner)
}

func TestResolve_BannerRejectsGeneratedOGImage(
	t *testing.T,
) {
	forge := &stubForge{files: map[string][]byte{}}
	page := hosts.RepoPage{CustomSocialImage: false, SocialImage: "https://opengraph.githubassets.com/generated.png"}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", page, nil)

	assert.Empty(t, media.Banner)
}

func TestResolve_BannerFallsBackToWideReadmeImage(
	t *testing.T,
) {
	readmeRaw := []byte("![Screenshot](shot.png)\n")
	forge := &stubForge{files: map[string][]byte{
		"shot.png": encodePNG(t, 1200, 400),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/shot.png", media.Banner)
}

func TestResolve_BannerFallsBackToScreenshotShapedImage(
	t *testing.T,
) {
	readmeRaw := []byte("![Screenshot](shot.png)\n")
	forge := &stubForge{files: map[string][]byte{
		"shot.png": encodePNG(t, 640, 360),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/shot.png", media.Banner)
}

func TestResolve_BannerNoneWhenNothingQualifies(
	t *testing.T,
) {
	readmeRaw := []byte("![Screenshot](shot.png)\n")
	forge := &stubForge{files: map[string][]byte{
		"shot.png": encodePNG(t, 100, 100),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, readmeRaw)

	assert.Empty(t, media.Banner)
}

func TestResolve_BannerSkipsBadgeImages(
	t *testing.T,
) {
	readmeRaw := []byte("![Build](https://img.shields.io/x.svg)\n![Screenshot](shot.png)\n")
	forge := &stubForge{files: map[string][]byte{
		"shot.png": encodePNG(t, 1200, 400),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/shot.png", media.Banner)
}

func TestResolve_SkipsUnresolvableExternalImage(
	t *testing.T,
) {
	readmeRaw := []byte("![Screenshot](https://someone-elses-cdn.example/shot.png)\n")
	forge := &stubForge{files: map[string][]byte{}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, readmeRaw)

	assert.Empty(t, media.Icon)
	assert.Empty(t, media.Banner)
}

func TestResolve_SkipsCandidateWhenFileMissing(
	t *testing.T,
) {
	readmeRaw := []byte("![Icon](missing.png)\n![Icon](icon-art.png)\n")
	forge := &stubForge{files: map[string][]byte{
		"icon-art.png": encodePNG(t, 300, 300),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/icon-art.png", media.Icon)
}

func TestResolve_SkipsCandidateWhenDataUnsniffable(
	t *testing.T,
) {
	readmeRaw := []byte("![Icon](garbage.png)\n![Icon](icon-art.png)\n")
	forge := &stubForge{files: map[string][]byte{
		"garbage.png":  []byte("not an image"),
		"icon-art.png": encodePNG(t, 300, 300),
	}}

	media := Resolve(context.Background(), forge, testNS, "v1.0.0", hosts.RepoPage{}, readmeRaw)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/icon-art.png", media.Icon)
}

func TestOwnerRepo_ParsesFromNamespace(
	t *testing.T,
) {
	owner, repo := ownerRepo(domain.Namespace("github.com/owner/repo@v1.0.0"))

	assert.Equal(t, "owner", owner)
	assert.Equal(t, "repo", repo)
}

func TestOwnerRepo_EmptyWhenTooFewSegments(
	t *testing.T,
) {
	owner, repo := ownerRepo(domain.Namespace("github.com/owner"))

	assert.Empty(t, owner)
	assert.Empty(t, repo)
}

func TestResolvePath_ProtocolRelativeRejected(
	t *testing.T,
) {
	_, ok := resolvePath("//cdn.example.com/img.png", "owner", "repo")

	assert.False(t, ok)
}

func TestResolvePath_RelativeIsCleaned(
	t *testing.T,
) {
	path, ok := resolvePath("./doc/logo.png", "owner", "repo")

	require.True(t, ok)
	assert.Equal(t, "doc/logo.png", path)
}

func TestResolvePath_SameRepoRawURLResolves(
	t *testing.T,
) {
	path, ok := resolvePath("https://raw.githubusercontent.com/owner/repo/main/doc/logo.png", "owner", "repo")

	require.True(t, ok)
	assert.Equal(t, "doc/logo.png", path)
}

func TestResolvePath_SameRepoBlobURLResolves(
	t *testing.T,
) {
	path, ok := resolvePath("https://github.com/owner/repo/blob/main/doc/logo.png", "owner", "repo")

	require.True(t, ok)
	assert.Equal(t, "doc/logo.png", path)
}

func TestResolvePath_MalformedSameRepoRawURLRejected(
	t *testing.T,
) {
	_, ok := resolvePath("https://raw.githubusercontent.com/owner/repo/main", "owner", "repo")

	assert.False(t, ok)
}

func TestResolvePath_OtherRepoURLRejected(
	t *testing.T,
) {
	_, ok := resolvePath("https://raw.githubusercontent.com/other/repo/main/x.png", "owner", "repo")

	assert.False(t, ok)
}

func TestFetchCapped_TruncatesAt64KiB(
	t *testing.T,
) {
	big := make([]byte, maxProbeBytes+1000)
	forge := &stubForge{files: map[string][]byte{"big.png": big}}

	data, ok := fetchCapped(context.Background(), forge, testNS, "v1", "big.png")

	require.True(t, ok)
	assert.Len(t, data, maxProbeBytes)
}

func TestFetchCapped_MissingFileFails(
	t *testing.T,
) {
	forge := &stubForge{files: map[string][]byte{}}

	data, ok := fetchCapped(context.Background(), forge, testNS, "v1", "missing.png")

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
