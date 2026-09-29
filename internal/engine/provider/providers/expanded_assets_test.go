package providers

import (
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const ripgrepExpandedAssetsURL = "https://github.com/BurntSushi/ripgrep/releases/expanded_assets/15.2.0"

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return body
}

func TestParseExpandedAssets_GoldenRipgrep(t *testing.T) {
	assets, err := parseExpandedAssets(readTestdata(t, "expanded_assets_ripgrep.html"), ripgrepExpandedAssetsURL)
	require.NoError(t, err)
	require.Len(t, assets, 4)

	assert.Equal(t, domain.ReleaseAsset{
		Name:   "ripgrep-15.2.0-aarch64-apple-darwin.tar.gz",
		URL:    "https://github.com/BurntSushi/ripgrep/releases/download/15.2.0/ripgrep-15.2.0-aarch64-apple-darwin.tar.gz",
		Digest: "sha256:3750b2e93f37e0c692657da574d7019a101c0084da05a790c83fd335bad973e4",
	}, assets[0])
	assert.Equal(t, domain.ReleaseAsset{
		Name:   "ripgrep-15.2.0-aarch64-apple-darwin.tar.gz.sha256",
		URL:    "https://github.com/BurntSushi/ripgrep/releases/download/15.2.0/ripgrep-15.2.0-aarch64-apple-darwin.tar.gz.sha256",
		Digest: "sha256:6548307715b72f409e4a2667fb1fc435822a2561a27445971b0925183313ebad",
	}, assets[1])
	for _, asset := range assets {
		assert.NotContains(t, asset.URL, "/archive/")
		assert.Contains(t, asset.URL, "https://github.com/")
	}
}

func TestParseExpandedAssets_Fragments(t *testing.T) {
	testCases := []struct {
		name string
		body string
		want []string
	}{
		{name: "valid empty list", body: `<div><ul data-view-component="true"></ul></div>`},
		{
			name: "only archive links",
			body: `<div><ul>
				<li><a href="/u/r/archive/refs/tags/v1.zip">Source code</a></li>
				<li><a href="/u/r/archive/refs/tags/v1.tar.gz">Source code</a></li>
			</ul></div>`,
		},
		{name: "item without a link", body: `<div><ul><li><span>no link here</span></li></ul></div>`},
		{name: "link without href", body: `<div><ul><li><a data-turbo="false">no href</a></li></ul></div>`},
		{
			name: "absolute href is kept",
			body: `<div><ul><li><a href="https://objects.githubusercontent.com/release/1/a.zip">a.zip</a></li></ul></div>`,
			want: []string{"https://objects.githubusercontent.com/release/1/a.zip"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assets, err := parseExpandedAssets([]byte(tc.body), ripgrepExpandedAssetsURL)

			require.NoError(t, err)
			urls := make([]string, 0, len(assets))
			for _, asset := range assets {
				urls = append(urls, asset.URL)
			}
			assert.Equal(t, append([]string{}, tc.want...), urls)
		})
	}
}

func TestParseExpandedAssets_UnexpectedPage(t *testing.T) {
	testCases := []struct {
		name    string
		body    string
		pageURL string
	}{
		{name: "no list", body: `<html><body><p>404 Not Found</p></body></html>`, pageURL: ripgrepExpandedAssetsURL},
		{name: "unparseable page url", body: `<div><ul><li><a href="/a.zip">a.zip</a></li></ul></div>`, pageURL: "://not a url"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseExpandedAssets([]byte(tc.body), tc.pageURL)

			assert.ErrorIs(t, err, ErrUnexpectedPage)
		})
	}
}

func TestResolveHref_UnparseableHref_ReturnsItUnchanged(t *testing.T) {
	base, err := url.Parse(ripgrepExpandedAssetsURL)
	require.NoError(t, err)

	assert.Equal(t, "://bad href", resolveHref(base, "://bad href"))
}
