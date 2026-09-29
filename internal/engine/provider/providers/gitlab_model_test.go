package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestGitLabAsset_NamesAndSecurity(t *testing.T) {
	testCases := []struct {
		name       string
		asset      gitlabAsset
		wantNames  []string
		wantSecure bool
	}{
		{
			name: "free text name plus both url basenames",
			asset: gitlabAsset{
				ReleaseAsset: domain.ReleaseAsset{Name: "Linux Build", URL: "https://gitlab.com/o/r/-/releases/v1/downloads/tool_linux.tar.gz"},
				linkURL:      "https://gitlab.com/api/v4/projects/1/packages/generic/tool/1/tool_linux%2Etar%2Egz",
			},
			wantNames:  []string{"linux build", "tool_linux.tar.gz"},
			wantSecure: true,
		},
		{
			name:      "unusable urls add nothing",
			asset:     gitlabAsset{ReleaseAsset: domain.ReleaseAsset{URL: "https://gitlab.com/"}, linkURL: "%zz"},
			wantNames: []string{},
		},
		{
			name:      "escaped basename is decoded and plain http is insecure",
			asset:     gitlabAsset{ReleaseAsset: domain.ReleaseAsset{Name: "a", URL: "https://x.test/%25zz"}, linkURL: "http://x.test/a"},
			wantNames: []string{"a", "%zz"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantNames, tc.asset.names())
			assert.Equal(t, tc.wantSecure, tc.asset.secure())
		})
	}
}

func TestGitLabLink_Asset_PrefersTheDirectAssetURL(t *testing.T) {
	direct := gitlabLink{Name: "a.zip", URL: "https://api.test/a", DirectAssetURL: "https://direct.test/a.zip"}.asset()
	plain := gitlabLink{Name: "a.zip", URL: "https://api.test/a.zip"}.asset()

	assert.Equal(t, "https://direct.test/a.zip", direct.URL)
	assert.Equal(t, "https://api.test/a", direct.linkURL)
	assert.Equal(t, "https://api.test/a.zip", plain.URL)
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
		{name: "wrong segment count", project: "1", path: "glab/file"},
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
		{name: "no project placeholder", template: "https://gitlab.com/api/v4/packages", link: "https://gitlab.com/api/v4/packages/generic/glab/1.0/glab.zip"},
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
