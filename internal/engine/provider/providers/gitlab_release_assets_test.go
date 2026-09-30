package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	glabBase            = "https://gitlab.com"
	glabNamespace       = "gitlab.com/gitlab-org/cli"
	glabTag             = "v1.119.0"
	glabReleaseURI      = "/api/v4/projects/gitlab-org%2Fcli/releases/v1.119.0"
	glabPackagesURI     = "/api/v4/projects/gitlab-org%2Fcli/packages?package_name=glab&package_type=generic&package_version=1.119.0"
	glabFilesURI        = "/api/v4/projects/gitlab-org%2Fcli/packages/70206788/package_files?page=1&per_page=100"
	glabChecksumsURI    = "/gitlab-org/cli/-/releases/v1.119.0/downloads/checksums.txt"
	glabChecksumsPkgURI = "/api/v4/projects/gitlab-org%2Fcli/packages/generic/glab/1%2E119%2E0/checksums%2Etxt"
)

type cannedResponse struct {
	status   int
	body     string
	headers  map[string]string
	location string
	hangup   bool
}

type fakeGitLab struct {
	doer *routedDoer
	down bool
}

func newFakeGitLab() *fakeGitLab {
	return &fakeGitLab{doer: &routedDoer{
		responses: map[string]fns.Response{},
		failures:  map[string]error{},
	}}
}

func (f *fakeGitLab) do(
	ctx context.Context,
	req fns.Request,
) (fns.Response, error) {
	if f.down {
		return fns.Response{}, errors.New("dial tcp: connection refused")
	}
	return f.doer.do(ctx, req)
}

func (f *fakeGitLab) route(
	uri string,
	resp cannedResponse,
) {
	delete(f.doer.failures, glabBase+uri)
	if resp.hangup {
		f.doer.failures[glabBase+uri] = errors.New("connection reset by peer")
		return
	}

	headers := http.Header{}
	for key, value := range resp.headers {
		headers.Set(key, value)
	}
	if resp.location != "" {
		headers.Set("Location", f.absolute(resp.location))
	}
	f.doer.responses[glabBase+uri] = fns.Response{Status: resp.status, Headers: headers, Body: []byte(resp.body)}
}

func (f *fakeGitLab) ok(
	uri string,
	body string,
) {
	f.route(uri, cannedResponse{status: http.StatusOK, body: body})
}

func (f *fakeGitLab) absolute(
	link string,
) string {
	if strings.HasPrefix(link, "/") {
		return glabBase + link
	}
	return link
}

func (f *fakeGitLab) hits() []string {
	f.doer.mu.Lock()
	defer f.doer.mu.Unlock()
	hits := make([]string, len(f.doer.requests))
	for i, url := range f.doer.requests {
		hits[i] = strings.TrimPrefix(url, glabBase)
	}
	return hits
}

func (f *fakeGitLab) count(
	uri string,
) int {
	n := 0
	for _, hit := range f.hits() {
		if hit == uri {
			n++
		}
	}
	return n
}

func (f *fakeGitLab) fixture(
	t *testing.T,
	name string,
) string {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return string(body)
}

func (f *fakeGitLab) serveGlab(
	t *testing.T,
) {
	t.Helper()
	f.ok(glabReleaseURI, f.fixture(t, "gitlab_release_glab.json"))
	f.ok(glabPackagesURI, f.fixture(t, "gitlab_packages_glab.json"))
	f.ok(glabFilesURI, f.fixture(t, "gitlab_package_files_glab.json"))
	f.route(glabChecksumsURI, cannedResponse{status: http.StatusFound, location: glabChecksumsPkgURI})
	f.ok(glabChecksumsPkgURI, f.fixture(t, "gitlab_checksums_glab.txt"))
}

func (f *fakeGitLab) onServer(
	link gitlabLink,
) gitlabLink {
	link.URL = f.absolute(link.URL)
	link.DirectAssetURL = f.absolute(link.DirectAssetURL)
	return link
}

func (f *fakeGitLab) config() Config {
	return Config{
		Host:           "gitlab.com",
		RawURL:         glabBase + "/{user}/{repo}/-/raw/{branch}/{file}",
		BlobURL:        glabBase + "/{user}/{repo}/-/blob/{branch}/{file}",
		ReleaseAPIURL:  glabBase + "/api/v4/projects/{user}%2F{repo}/releases/{tag}",
		RepoPageURL:    glabBase + "/{user}/{repo}",
		PackagesAPIURL: glabBase + "/api/v4/projects/{project}/packages",
		Timeout:        5 * time.Second,
		Do:             f.do,
	}
}

func (f *fakeGitLab) provider() Provider {
	return NewGitLab(f.config())
}

const (
	glabLinuxTarDigest   = "sha256:4d83375d202ffa634eaf627fd9272b610fa599f20f5b711f55d770583eca2b84"
	glabDarwinTarDigest  = "sha256:d9cddd1dbe9a8bea8d5709f90a9ac13c3dd3eb0d0e94e3bc4fae6b27abd6a3db"
	glabWindowsZipDigest = "sha256:fa12c31cf8100fd5b41f6fc9cf907b344474d183c766d9b6806dbbbeba14e7af"
	glabLinuxDebDigest   = "sha256:95a9ca105518dec7f70ee403d98501402a5fda154c406f4f4cd6696928d3a098"
	glabChecksumsDigest  = "sha256:c484749df6e962ba86efa158a8c5208533b10ffda1228e8639009a288192c399"
	otherDigestHex       = "1111111111111111111111111111111111111111111111111111111111111111"
)

func glabAssets(
	t *testing.T,
	f *fakeGitLab,
) map[string]domain.ReleaseAsset {
	t.Helper()
	assets, err := f.provider().ReleaseAssets(context.Background(), domain.Namespace(glabNamespace), glabTag)
	require.NoError(t, err)

	byName := make(map[string]domain.ReleaseAsset, len(assets))
	for _, asset := range assets {
		byName[asset.Name] = asset
	}
	require.Len(t, byName, len(assets))
	return byName
}

func digestsOf(
	assets map[string]domain.ReleaseAsset,
) map[string]string {
	digests := make(map[string]string, len(assets))
	for name, asset := range assets {
		digests[name] = asset.Digest
	}
	return digests
}

func linkJSON(
	t *testing.T,
	links ...gitlabLink,
) string {
	t.Helper()
	var release gitlabRelease
	release.Assets.Links = links
	body, err := json.Marshal(release)
	require.NoError(t, err)
	return string(body)
}

func TestGitLab_ReleaseAssets_PackageFilesDigestEveryLink(t *testing.T) {
	f := newFakeGitLab()
	f.serveGlab(t)

	assets := glabAssets(t, f)

	assert.Equal(t, map[string]string{
		"glab_1.119.0_linux_amd64.tar.gz":  glabLinuxTarDigest,
		"glab_1.119.0_darwin_arm64.tar.gz": glabDarwinTarDigest,
		"glab_1.119.0_windows_amd64.zip":   glabWindowsZipDigest,
		"glab_1.119.0_linux_amd64.deb":     glabLinuxDebDigest,
		"checksums.txt":                    glabChecksumsDigest,
	}, digestsOf(assets))
	assert.Equal(t, 1, f.count(glabPackagesURI))
	assert.Equal(t, 1, f.count(glabFilesURI))
	assert.Zero(t, f.count(glabChecksumsURI))
	assert.Equal(t, glabBase+"/gitlab-org/cli/-/releases/v1.119.0/downloads/glab_1.119.0_linux_amd64.tar.gz", assets["glab_1.119.0_linux_amd64.tar.gz"].URL)
	for _, asset := range assets {
		assert.NotContains(t, asset.URL, "/-/archive/")
	}
}

func TestGitLab_ReleaseAssets_FallsBackToTheChecksumFile(t *testing.T) {
	testCases := []struct {
		name     string
		packages cannedResponse
	}{
		{name: "package api 404", packages: cannedResponse{status: http.StatusNotFound}},
		{name: "no package published", packages: cannedResponse{status: http.StatusOK, body: "[]"}},
		{name: "only a fuzzy name match", packages: cannedResponse{
			status: http.StatusOK,
			body:   `[{"id": 1, "name": "glab-nightly", "version": "1.119.0"}]`,
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitLab()
			f.serveGlab(t)
			f.route(glabPackagesURI, tc.packages)

			assets := glabAssets(t, f)

			assert.Equal(t, map[string]string{
				"glab_1.119.0_linux_amd64.tar.gz":  glabLinuxTarDigest,
				"glab_1.119.0_darwin_arm64.tar.gz": glabDarwinTarDigest,
				"glab_1.119.0_windows_amd64.zip":   glabWindowsZipDigest,
				"glab_1.119.0_linux_amd64.deb":     glabLinuxDebDigest,
				"checksums.txt":                    "",
			}, digestsOf(assets))
			assert.Zero(t, f.count(glabFilesURI))
			assert.Equal(t, 1, f.count(glabChecksumsPkgURI))
		})
	}
}

func TestGitLab_ReleaseAssets_PackageDigestWinsOverTheChecksumFile(t *testing.T) {
	f := newFakeGitLab()
	f.serveGlab(t)
	f.ok(glabFilesURI, `[{"file_name": "glab_1.119.0_linux_amd64.tar.gz", "file_sha256": "`+otherDigestHex+`"}]`)

	assets := glabAssets(t, f)

	assert.Equal(t, "sha256:"+otherDigestHex, assets["glab_1.119.0_linux_amd64.tar.gz"].Digest)
	assert.Equal(t, glabDarwinTarDigest, assets["glab_1.119.0_darwin_arm64.tar.gz"].Digest)
	assert.Empty(t, assets["checksums.txt"].Digest)
}

func TestGitLab_ReleaseAssets_NoDigestSourceLeavesDigestsEmpty(t *testing.T) {
	f := newFakeGitLab()
	f.ok(glabReleaseURI, linkJSON(t,
		gitlabLink{Name: "tool_linux_amd64.tar.gz", URL: "https://downloads.example.test/tool_linux_amd64.tar.gz"},
		gitlabLink{Name: "tool_darwin_arm64.tar.gz", URL: "https://downloads.example.test/tool_darwin_arm64.tar.gz"},
	))

	assets := glabAssets(t, f)

	assert.Equal(t, map[string]string{
		"tool_linux_amd64.tar.gz":  "",
		"tool_darwin_arm64.tar.gz": "",
	}, digestsOf(assets))
	assert.Equal(t, "https://downloads.example.test/tool_linux_amd64.tar.gz", assets["tool_linux_amd64.tar.gz"].URL)
	assert.Equal(t, []string{glabReleaseURI}, f.hits())
}

func TestGitLab_ReleaseAssets_PagesThroughPackageFiles(t *testing.T) {
	f := newFakeGitLab()
	f.serveGlab(t)
	fullPage := make([]gitlabPackageFile, packageFilesPerPage)
	for i := range fullPage {
		fullPage[i] = gitlabPackageFile{FileName: fmt.Sprintf("filler-%d", i), FileSHA256: otherDigestHex}
	}
	body, err := json.Marshal(fullPage)
	require.NoError(t, err)
	f.ok(glabFilesURI, string(body))
	f.ok(strings.Replace(glabFilesURI, "page=1", "page=2", 1), f.fixture(t, "gitlab_package_files_glab.json"))

	assets := glabAssets(t, f)

	assert.Equal(t, glabLinuxTarDigest, assets["glab_1.119.0_linux_amd64.tar.gz"].Digest)
}

func TestGitLab_ReleaseAssets_StopsPagingAtTheCap(t *testing.T) {
	f := newFakeGitLab()
	f.serveGlab(t)
	fullPage := make([]gitlabPackageFile, packageFilesPerPage)
	for i := range fullPage {
		fullPage[i] = gitlabPackageFile{FileName: fmt.Sprintf("filler-%d", i), FileSHA256: "not-a-digest"}
	}
	body, err := json.Marshal(fullPage)
	require.NoError(t, err)
	for page := 1; page <= maxPackageFilePages+1; page++ {
		f.ok(strings.Replace(glabFilesURI, "page=1", fmt.Sprintf("page=%d", page), 1), string(body))
	}

	_ = glabAssets(t, f)

	assert.Zero(t, f.count(strings.Replace(glabFilesURI, "page=1", fmt.Sprintf("page=%d", maxPackageFilePages+1), 1)))
	assert.Equal(t, 1, f.count(strings.Replace(glabFilesURI, "page=1", fmt.Sprintf("page=%d", maxPackageFilePages), 1)))
}

func TestGitLab_ReleaseAssets_404_ReturnsNoAssets(t *testing.T) {
	f := newFakeGitLab()

	assets, err := f.provider().ReleaseAssets(context.Background(), domain.Namespace(glabNamespace), glabTag)

	require.NoError(t, err)
	assert.Empty(t, assets)
}

func TestGitLab_ReleaseAssets_FailureMapping(t *testing.T) {
	testCases := []struct {
		name         string
		uri          string
		resp         cannedResponse
		rateLimited  bool
		unauthorized bool
		unexpected   bool
	}{
		{
			name:        "release rate limited",
			uri:         glabReleaseURI,
			resp:        cannedResponse{status: http.StatusTooManyRequests, headers: map[string]string{"Retry-After": "30"}},
			rateLimited: true,
		},
		{name: "release unauthorized", uri: glabReleaseURI, resp: cannedResponse{status: http.StatusUnauthorized}, unauthorized: true},
		{name: "release malformed", uri: glabReleaseURI, resp: cannedResponse{status: http.StatusOK, body: "{"}, unexpected: true},
		{name: "release hangup", uri: glabReleaseURI, resp: cannedResponse{hangup: true}},
		{name: "packages rate limited", uri: glabPackagesURI, resp: cannedResponse{status: http.StatusTooManyRequests}, rateLimited: true},
		{name: "package files server error", uri: glabFilesURI, resp: cannedResponse{status: http.StatusBadGateway}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitLab()
			f.serveGlab(t)
			f.route(tc.uri, tc.resp)

			_, err := f.provider().ReleaseAssets(context.Background(), domain.Namespace(glabNamespace), glabTag)

			require.Error(t, err)
			assertFailureKind(t, err, tc.rateLimited, tc.unauthorized, tc.unexpected)
		})
	}
}

func TestGitLab_ReleaseAssets_ChecksumFailuresAreMisses(t *testing.T) {
	testCases := []struct {
		name string
		resp cannedResponse
	}{
		{name: "rate limited", resp: cannedResponse{status: http.StatusTooManyRequests}},
		{name: "not found", resp: cannedResponse{status: http.StatusNotFound}},
		{name: "oversized", resp: cannedResponse{status: http.StatusOK, body: strings.Repeat("#", maxChecksumBytes+1)}},
		{name: "hangup", resp: cannedResponse{hangup: true}},
		{name: "redirect without a location", resp: cannedResponse{status: http.StatusFound}},
		{name: "redirect to plain http", resp: cannedResponse{status: http.StatusFound, location: "http://downloads.example.test/checksums.txt"}},
		{name: "redirect loop", resp: cannedResponse{status: http.StatusFound, location: glabChecksumsPkgURI}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitLab()
			f.serveGlab(t)
			f.route(glabPackagesURI, cannedResponse{status: http.StatusNotFound})
			f.route(glabChecksumsPkgURI, tc.resp)

			assets := glabAssets(t, f)

			for _, asset := range assets {
				assert.Empty(t, asset.Digest, asset.Name)
			}
		})
	}
}

func TestGitLab_ReleaseAssets_RedirectHopsAreCapped(t *testing.T) {
	f := newFakeGitLab()
	f.serveGlab(t)
	f.route(glabPackagesURI, cannedResponse{status: http.StatusNotFound})
	f.route(glabChecksumsPkgURI, cannedResponse{status: http.StatusFound, location: glabChecksumsPkgURI})

	_ = glabAssets(t, f)

	assert.Equal(t, 1, f.count(glabChecksumsURI))
	assert.Equal(t, maxChecksumRedirects, f.count(glabChecksumsPkgURI))
}

func TestGitLab_ReleaseAssets_DeniedPackageLookupsFallBackToTheChecksumFile(t *testing.T) {
	testCases := []struct {
		name string
		uri  string
		resp cannedResponse
	}{
		{name: "packages unauthorized", uri: glabPackagesURI, resp: cannedResponse{status: http.StatusUnauthorized}},
		{name: "package files forbidden", uri: glabFilesURI, resp: cannedResponse{status: http.StatusForbidden}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitLab()
			f.serveGlab(t)
			f.route(tc.uri, tc.resp)

			assets := glabAssets(t, f)

			assert.Equal(t, glabLinuxTarDigest, assets["glab_1.119.0_linux_amd64.tar.gz"].Digest)
			assert.Equal(t, glabWindowsZipDigest, assets["glab_1.119.0_windows_amd64.zip"].Digest)
			assert.Equal(t, 1, f.count(glabChecksumsPkgURI))
		})
	}
}

func TestGitLab_ReleaseAssets_PlainHTTPLinksNeverCarryADigest(t *testing.T) {
	f := newFakeGitLab()
	f.serveGlab(t)
	secureDirect := glabBase + "/gitlab-org/cli/-/releases/v1.119.0/downloads/glab_1.119.0_linux_amd64.tar.gz"
	release := strings.Replace(
		f.fixture(t, "gitlab_release_glab.json"),
		secureDirect,
		"http://downloads.example.test/glab_1.119.0_linux_amd64.tar.gz",
		1,
	)
	f.ok(glabReleaseURI, release)

	assets := glabAssets(t, f)

	assert.Empty(t, assets["glab_1.119.0_linux_amd64.tar.gz"].Digest)
	assert.Equal(t, glabDarwinTarDigest, assets["glab_1.119.0_darwin_arm64.tar.gz"].Digest)
}

func TestGitLab_ReleaseAssets_ChecksumFileMatching(t *testing.T) {
	testCases := []struct {
		name  string
		files map[string]string
		links func(f *fakeGitLab) []gitlabLink
		want  map[string]string
	}{
		{
			name:  "single asset sum file",
			files: map[string]string{"/files/Tool.AppImage.sha256": otherDigestHex + "\n"},
			links: func(f *fakeGitLab) []gitlabLink {
				return []gitlabLink{
					f.onServer(gitlabLink{Name: "Tool.AppImage", URL: "/files/Tool.AppImage"}),
					f.onServer(gitlabLink{Name: "Tool.AppImage.sha256", URL: "/files/Tool.AppImage.sha256"}),
				}
			},
			want: map[string]string{"Tool.AppImage": "sha256:" + otherDigestHex, "Tool.AppImage.sha256": ""},
		},
		{
			name:  "matches by url basename",
			files: map[string]string{"/files/sha256sums.txt": otherDigestHex + "  tool_linux_amd64.tar.gz\n"},
			links: func(f *fakeGitLab) []gitlabLink {
				return []gitlabLink{
					{Name: "Linux build", URL: "https://downloads.example.test/tool_linux_amd64.tar.gz"},
					f.onServer(gitlabLink{Name: "Checksums", URL: "/files/sha256sums.txt"}),
				}
			},
			want: map[string]string{"Linux build": "sha256:" + otherDigestHex, "Checksums": ""},
		},
		{
			name: "conflicting sum files drop the digest",
			files: map[string]string{
				"/files/checksums.txt":      sampleHex + "  tool.tar.gz\n",
				"/files/tool.tar.gz.sha256": otherDigestHex + "\n",
			},
			links: func(f *fakeGitLab) []gitlabLink {
				return []gitlabLink{
					{Name: "tool.tar.gz", URL: "https://downloads.example.test/tool.tar.gz"},
					f.onServer(gitlabLink{Name: "checksums.txt", URL: "/files/checksums.txt"}),
					f.onServer(gitlabLink{Name: "tool.tar.gz.sha256", URL: "/files/tool.tar.gz.sha256"}),
				}
			},
			want: map[string]string{"tool.tar.gz": "", "checksums.txt": "", "tool.tar.gz.sha256": ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitLab()
			for uri, body := range tc.files {
				f.ok(uri, body)
			}
			f.ok(glabReleaseURI, linkJSON(t, tc.links(f)...))

			assert.Equal(t, tc.want, digestsOf(glabAssets(t, f)))
		})
	}
}

func TestGitLab_ReleaseAssets_ChecksumFileSecurity(t *testing.T) {
	testCases := []struct {
		name       string
		asset      gitlabLink
		sums       gitlabLink
		wantDigest string
		wantFetch  bool
	}{
		{
			name:       "https asset and sum file",
			asset:      gitlabLink{Name: "tool.tar.gz", URL: "https://downloads.example.test/tool.tar.gz"},
			sums:       gitlabLink{Name: "checksums.txt", URL: "/files/checksums.txt"},
			wantDigest: "sha256:" + otherDigestHex,
			wantFetch:  true,
		},
		{
			name:  "plain http asset link",
			asset: gitlabLink{Name: "tool.tar.gz", URL: "http://downloads.example.test/tool.tar.gz"},
			sums:  gitlabLink{Name: "checksums.txt", URL: "/files/checksums.txt"},
		},
		{
			name:  "plain http sum file link",
			asset: gitlabLink{Name: "tool.tar.gz", URL: "https://downloads.example.test/tool.tar.gz"},
			sums: gitlabLink{
				Name:           "checksums.txt",
				URL:            "http://downloads.example.test/checksums.txt",
				DirectAssetURL: "/files/checksums.txt",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeGitLab()
			f.ok("/files/checksums.txt", otherDigestHex+"  tool.tar.gz\n")
			f.ok(glabReleaseURI, linkJSON(t, tc.asset, f.onServer(tc.sums)))

			assets := glabAssets(t, f)

			assert.Equal(t, tc.wantDigest, assets["tool.tar.gz"].Digest)
			assert.Equal(t, tc.wantFetch, f.count("/files/checksums.txt") == 1)
		})
	}
}

func TestGitLab_ReleaseAssets_OnlyRelevantSumFilesAreFetched(t *testing.T) {
	f := newFakeGitLab()
	f.ok("/files/other.tar.gz.sha256", otherDigestHex+"\n")
	f.ok("/files/tool.tar.gz.sha256", otherDigestHex+"\n")
	f.ok(glabReleaseURI, linkJSON(t,
		gitlabLink{Name: "tool.tar.gz", URL: "https://downloads.example.test/tool.tar.gz"},
		f.onServer(gitlabLink{Name: "tool.tar.gz.sha256", URL: "/files/tool.tar.gz.sha256"}),
		f.onServer(gitlabLink{Name: "other.tar.gz.sha256", URL: "/files/other.tar.gz.sha256"}),
	))

	assets := glabAssets(t, f)

	assert.Equal(t, "sha256:"+otherDigestHex, assets["tool.tar.gz"].Digest)
	assert.Equal(t, 1, f.count("/files/tool.tar.gz.sha256"))
	assert.Zero(t, f.count("/files/other.tar.gz.sha256"))
}

func assertFailureKind(
	t *testing.T,
	err error,
	rateLimited bool,
	unauthorized bool,
	unexpected bool,
) {
	t.Helper()
	var limited *RateLimitedError
	var denied *UnauthorizedError
	assert.Equal(t, rateLimited, errors.As(err, &limited))
	assert.Equal(t, unauthorized, errors.As(err, &denied))
	assert.Equal(t, unexpected, errors.Is(err, ErrUnexpectedPage))
}

func TestGitLab_ReleaseAssets_EscapesTheTag(t *testing.T) {
	f := newFakeGitLab()

	_, _ = f.provider().ReleaseAssets(context.Background(), domain.Namespace(glabNamespace), "release/v1")

	assert.Equal(t, 1, f.count("/api/v4/projects/gitlab-org%2Fcli/releases/release%2Fv1"))
}

func TestGitLab_ReleaseAssets_Unreachable(t *testing.T) {
	f := newFakeGitLab()
	provider := f.provider()
	f.down = true

	_, err := provider.ReleaseAssets(context.Background(), domain.Namespace(glabNamespace), glabTag)
	require.Error(t, err)
}

func TestGitLab_ReleaseAssets_Misconfigured(t *testing.T) {
	f := newFakeGitLab()
	testCases := []struct {
		name    string
		ns      domain.Namespace
		cfg     func(Config) Config
		wantErr error
	}{
		{
			name:    "no release api url",
			ns:      domain.Namespace(glabNamespace),
			cfg:     func(c Config) Config { c.ReleaseAPIURL = ""; return c },
			wantErr: ErrNoRawURL,
		},
		{
			name: "invalid namespace",
			ns:   domain.Namespace("only-two"),
			cfg:  func(c Config) Config { return c },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewGitLab(tc.cfg(f.config()))

			_, assetsErr := provider.ReleaseAssets(context.Background(), tc.ns, glabTag)

			require.Error(t, assetsErr)
			if tc.wantErr != nil {
				assert.ErrorIs(t, assetsErr, tc.wantErr)
			}
			assert.Empty(t, f.hits())
		})
	}
}

func TestGitLab_ReleaseAssets_NoPackagesTemplateSkipsThePackageLookup(t *testing.T) {
	f := newFakeGitLab()
	f.serveGlab(t)
	cfg := f.config()
	cfg.PackagesAPIURL = ""

	assets, err := NewGitLab(cfg).ReleaseAssets(context.Background(), domain.Namespace(glabNamespace), glabTag)

	require.NoError(t, err)
	require.Len(t, assets, 5)
	assert.Zero(t, f.count(glabPackagesURI))
	assert.Equal(t, 1, f.count(glabChecksumsPkgURI))
}
