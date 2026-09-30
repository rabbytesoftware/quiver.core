package media

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	gitlabNS           = domain.Namespace("gitlab.com/owner/repo@v1.0.0")
	gitlabRawTemplate  = "https://gitlab.com/owner/repo/-/raw/{branch}/{file}"
	gitlabBlobTemplate = "https://gitlab.com/owner/repo/-/blob/{branch}/{file}"
	squareSVG          = `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"></svg>`
)

func gitlabHost(
	files map[string][]byte,
) *stubHost {
	return &stubHost{
		files:        files,
		rawTemplate:  gitlabRawTemplate,
		blobTemplate: gitlabBlobTemplate,
	}
}

func TestResolve_GitLabHostPinsToItsOwnRawURL(t *testing.T) {
	testCases := []struct {
		name       string
		withIcon   bool
		readme     string
		wantIcon   string
		wantBanner string
	}{
		{
			name:     "probed icon",
			withIcon: true,
			wantIcon: "https://gitlab.com/owner/repo/-/raw/v1.0.0/logo.svg",
		},
		{
			name:       "relative readme banner",
			readme:     "![banner](docs/banner.png)\n",
			wantBanner: "https://gitlab.com/owner/repo/-/raw/v1.0.0/docs/banner.png",
		},
		{
			name:       "absolute raw url at another ref",
			readme:     "![banner](https://gitlab.com/owner/repo/-/raw/main/docs/banner.png)\n",
			wantBanner: "https://gitlab.com/owner/repo/-/raw/v1.0.0/docs/banner.png",
		},
		{
			name:       "absolute blob url",
			readme:     "![banner](https://gitlab.com/owner/repo/-/blob/main/docs/banner.png)\n",
			wantBanner: "https://gitlab.com/owner/repo/-/raw/v1.0.0/docs/banner.png",
		},
		{
			name:   "github url is not this repository",
			readme: "![banner](https://raw.githubusercontent.com/owner/repo/main/docs/banner.png)\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string][]byte{"docs/banner.png": encodePNG(t, 1200, 400)}
			if tc.withIcon {
				files = map[string][]byte{"logo.svg": []byte(squareSVG)}
			}

			media := resolveAll(context.Background(), gitlabHost(files), gitlabNS, "", []byte(tc.readme))

			assert.Equal(t, tc.wantIcon, media.Icon)
			assert.Equal(t, tc.wantBanner, media.Banner)
		})
	}
}

func TestResolve_HostWithoutRawURLsPinsNothing(t *testing.T) {
	host := &stubHost{
		files:       map[string][]byte{"logo.svg": []byte(squareSVG)},
		noTemplates: true,
	}

	media := resolveAll(context.Background(), host, testNS, "", []byte("![s](logo.svg)\n"))

	assert.Empty(t, media.Icon)
	assert.Empty(t, media.Banner)
}

func TestPathUnder(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		template string
		wantPath string
		wantOK   bool
	}{
		{
			name:     "any ref",
			src:      "https://gitlab.com/o/r/-/raw/feature-x/a/b.png",
			template: "https://gitlab.com/o/r/-/raw/{ref}/{file}",
			wantPath: "a/b.png",
			wantOK:   true,
		},
		{name: "no template", src: "https://gitlab.com/o/r/-/raw/main/a.png"},
		{name: "template without the ref", src: "https://x.test/a.png", template: "https://x.test/{file}"},
		{name: "ref at the start", src: "main/a.png", template: "{ref}/{file}"},
		{name: "template without a file", src: "https://x.test/main/a.png", template: "https://x.test/{ref}"},
		{name: "file right after the ref", src: "https://x.test/main/a.png", template: "https://x.test/{ref}{file}"},
		{name: "other prefix", src: "https://y.test/main/a.png", template: "https://x.test/{ref}/{file}"},
		{name: "missing separator", src: "https://x.test/main", template: "https://x.test/{ref}/{file}"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path, ok := pathUnder(tc.src, tc.template)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantPath, path)
		})
	}
}
