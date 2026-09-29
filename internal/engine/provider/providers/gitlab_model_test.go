package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestGitLabAsset_Names(t *testing.T) {
	testCases := []struct {
		name  string
		asset gitlabAsset
		want  []string
	}{
		{
			name: "free text name plus both url basenames",
			asset: gitlabAsset{
				ReleaseAsset: domain.ReleaseAsset{Name: "Linux Build", URL: "https://gitlab.com/o/r/-/releases/v1/downloads/tool_linux.tar.gz"},
				linkURL:      "https://gitlab.com/api/v4/projects/1/packages/generic/tool/1/tool_linux%2Etar%2Egz",
			},
			want: []string{"linux build", "tool_linux.tar.gz"},
		},
		{
			name:  "unusable urls add nothing",
			asset: gitlabAsset{ReleaseAsset: domain.ReleaseAsset{URL: "https://gitlab.com/"}, linkURL: "%zz"},
			want:  []string{},
		},
		{
			name:  "escaped basename is decoded",
			asset: gitlabAsset{ReleaseAsset: domain.ReleaseAsset{Name: "a", URL: "https://x.test/%25zz"}},
			want:  []string{"a", "%zz"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.asset.names())
		})
	}
}

func TestGitLabAsset_Secure(t *testing.T) {
	testCases := []struct {
		name  string
		asset gitlabAsset
		want  bool
	}{
		{name: "both https", asset: gitlabAsset{ReleaseAsset: domain.ReleaseAsset{URL: "https://a.test/x"}, linkURL: "https://b.test/x"}, want: true},
		{name: "plain http link", asset: gitlabAsset{ReleaseAsset: domain.ReleaseAsset{URL: "https://a.test/x"}, linkURL: "http://b.test/x"}},
		{name: "plain http download", asset: gitlabAsset{ReleaseAsset: domain.ReleaseAsset{URL: "http://a.test/x"}, linkURL: "https://b.test/x"}},
		{name: "hostless", asset: gitlabAsset{ReleaseAsset: domain.ReleaseAsset{URL: "https:///x"}, linkURL: "https://b.test/x"}},
		{name: "unparseable", asset: gitlabAsset{ReleaseAsset: domain.ReleaseAsset{URL: "https://a.test/x"}, linkURL: "%zz"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.asset.secure())
		})
	}
}

func TestPublicAssets_DropsDigestsOfInsecureAssets(t *testing.T) {
	assets := []gitlabAsset{
		{ReleaseAsset: domain.ReleaseAsset{Name: "a", URL: "https://a.test/a", Digest: "sha256:a"}, linkURL: "https://a.test/a"},
		{ReleaseAsset: domain.ReleaseAsset{Name: "b", URL: "https://a.test/b", Digest: "sha256:b"}, linkURL: "http://a.test/b"},
	}

	assert.Equal(t, []domain.ReleaseAsset{
		{Name: "a", URL: "https://a.test/a", Digest: "sha256:a"},
		{Name: "b", URL: "https://a.test/b"},
	}, publicAssets(assets))
}

func TestGitLabLink_Asset(t *testing.T) {
	testCases := []struct {
		name    string
		link    gitlabLink
		wantURL string
	}{
		{
			name:    "prefers the direct asset url",
			link:    gitlabLink{Name: "a.zip", URL: "https://api.test/a", DirectAssetURL: "https://direct.test/a.zip"},
			wantURL: "https://direct.test/a.zip",
		},
		{
			name:    "falls back to the link url",
			link:    gitlabLink{Name: "a.zip", URL: "https://api.test/a.zip"},
			wantURL: "https://api.test/a.zip",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asset := tc.link.asset()

			assert.Equal(t, domain.ReleaseAsset{Name: "a.zip", URL: tc.wantURL}, asset.ReleaseAsset)
			assert.Equal(t, tc.link.URL, asset.linkURL)
		})
	}
}

func TestAbsoluteURL(t *testing.T) {
	testCases := []struct {
		name string
		base string
		href string
		want string
	}{
		{name: "relative against the api base", base: "https://gitlab.com/api/v4/projects/1", href: "/uploads/a.png", want: "https://gitlab.com/uploads/a.png"},
		{name: "absolute stays", base: "https://gitlab.com/api", href: "https://cdn.test/a.png", want: "https://cdn.test/a.png"},
		{name: "empty href", base: "https://gitlab.com/api"},
		{name: "unparseable base", base: "%zz", href: "/uploads/a.png", want: "/uploads/a.png"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, absoluteURL(tc.base, tc.href))
		})
	}
}

func TestGenericPackageFile(t *testing.T) {
	testCases := []struct {
		name     string
		project  string
		path     string
		wantRef  packageRef
		wantFile string
		wantOK   bool
	}{
		{
			name:     "escaped segments",
			project:  "gitlab-org%2Fcli",
			path:     "glab/1%2E119%2E0/glab_1%2E119%2E0_linux_amd64%2Etar%2Egz",
			wantRef:  packageRef{project: "gitlab-org%2Fcli", name: "glab", version: "1.119.0"},
			wantFile: "glab_1.119.0_linux_amd64.tar.gz",
			wantOK:   true,
		},
		{name: "empty project", project: "", path: "glab/1.0/file"},
		{name: "too few segments", project: "1", path: "glab/file"},
		{name: "too many segments", project: "1", path: "glab/1.0/sub/file"},
		{name: "bad escape", project: "1", path: "glab/1.0/%zz"},
		{name: "empty file", project: "1", path: "glab/1.0/"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ref, file, ok := genericPackageFile(tc.project, tc.path)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantRef, ref)
				assert.Equal(t, tc.wantFile, file)
			}
		})
	}
}

func TestPackageRef_Match(t *testing.T) {
	ref := packageRef{name: "glab", version: "1.119.0"}
	testCases := []struct {
		name     string
		packages []gitlabPackage
		wantID   int64
		wantOK   bool
	}{
		{
			name: "exact match among fuzzy hits",
			packages: []gitlabPackage{
				{ID: 1, Name: "glab-nightly", Version: "1.119.0"},
				{ID: 2, Name: "glab", Version: "1.119.0-rc1"},
				{ID: 3, Name: "glab", Version: "1.119.0"},
			},
			wantID: 3,
			wantOK: true,
		},
		{name: "no packages"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			id, ok := ref.match(tc.packages)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantID, id)
		})
	}
}

func TestGitLab_PackageFileOf(t *testing.T) {
	testCases := []struct {
		name     string
		template string
		link     string
		wantOK   bool
	}{
		{
			name:     "generic package link",
			template: "https://gitlab.com/api/v4/projects/{project}/packages",
			link:     "https://gitlab.com/api/v4/projects/34675721/packages/generic/glab/1.0/glab.zip",
			wantOK:   true,
		},
		{
			name:     "template without a project placeholder",
			template: "https://gitlab.com/api/v4/packages",
			link:     "https://gitlab.com/api/v4/packages/generic/glab/1.0/glab.zip",
		},
		{
			name:     "link on another host",
			template: "https://gitlab.com/api/v4/projects/{project}/packages",
			link:     "https://downloads.example.test/api/v4/projects/1/packages/generic/glab/1.0/glab.zip",
		},
		{
			name:     "non generic package",
			template: "https://gitlab.com/api/v4/projects/{project}/packages",
			link:     "https://gitlab.com/api/v4/projects/1/packages/npm/glab/-/glab-1.0.tgz",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewGitLab(Config{Host: "gitlab.com", PackagesAPIURL: tc.template}).(*gitlabProvider)

			_, _, ok := p.packageFileOf(tc.link)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}
