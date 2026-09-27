package providers

import (
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ripgrepExpandedAssetsURL = "https://github.com/BurntSushi/ripgrep/releases/expanded_assets/15.2.0"

const crowbarExpandedAssetsURL = "https://github.com/char2cs/crowbar/releases/expanded_assets/nightly"

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return body
}

func TestParseExpandedAssets_GoldenRipgrep_ParsesNamesURLsSizesAndDigests(t *testing.T) {
	assets, err := parseExpandedAssets(readTestdata(t, "expanded_assets_ripgrep.html"), ripgrepExpandedAssetsURL)
	require.NoError(t, err)
	require.Len(t, assets, 4)

	assert.Equal(t, Asset{
		Name:   "ripgrep-15.2.0-aarch64-apple-darwin.tar.gz",
		URL:    "https://github.com/BurntSushi/ripgrep/releases/download/15.2.0/ripgrep-15.2.0-aarch64-apple-darwin.tar.gz",
		Size:   1761607,
		Digest: "sha256:3750b2e93f37e0c692657da574d7019a101c0084da05a790c83fd335bad973e4",
	}, assets[0])

	assert.Equal(t, Asset{
		Name:   "ripgrep-15.2.0-aarch64-apple-darwin.tar.gz.sha256",
		URL:    "https://github.com/BurntSushi/ripgrep/releases/download/15.2.0/ripgrep-15.2.0-aarch64-apple-darwin.tar.gz.sha256",
		Size:   109,
		Digest: "sha256:6548307715b72f409e4a2667fb1fc435822a2561a27445971b0925183313ebad",
	}, assets[1])
}

func TestParseExpandedAssets_GoldenRipgrep_ExcludesSourceCodeArchiveLinks(t *testing.T) {
	assets, err := parseExpandedAssets(readTestdata(t, "expanded_assets_ripgrep.html"), ripgrepExpandedAssetsURL)
	require.NoError(t, err)

	for _, asset := range assets {
		assert.NotContains(t, asset.URL, "/archive/")
	}
}

func TestParseExpandedAssets_GoldenRipgrep_URLsAreAbsolute(t *testing.T) {
	assets, err := parseExpandedAssets(readTestdata(t, "expanded_assets_ripgrep.html"), ripgrepExpandedAssetsURL)
	require.NoError(t, err)

	for _, asset := range assets {
		assert.Contains(t, asset.URL, "https://github.com/")
	}
}

func TestParseExpandedAssets_GoldenCrowbarNightly_Parses(t *testing.T) {
	assets, err := parseExpandedAssets(readTestdata(t, "expanded_assets_crowbar_nightly.html"), crowbarExpandedAssetsURL)
	require.NoError(t, err)
	require.Len(t, assets, 4)

	assert.Equal(t, "crowbar-api-darwin-amd64", assets[0].Name)
	assert.Equal(t, "https://github.com/char2cs/crowbar/releases/download/nightly/crowbar-api-darwin-amd64", assets[0].URL)
	assert.Equal(t, int64(97936998), assets[0].Size)
}

func TestParseExpandedAssets_ValidEmptyList_ReturnsEmptySlice(t *testing.T) {
	body := `<div class="Box Box--condensed"><ul data-view-component="true"></ul></div>`

	assets, err := parseExpandedAssets([]byte(body), ripgrepExpandedAssetsURL)
	require.NoError(t, err)
	assert.Empty(t, assets)
}

func TestParseExpandedAssets_OnlyArchiveLinks_ReturnsEmptySlice(t *testing.T) {
	body := `<div><ul>
		<li><a href="/u/r/archive/refs/tags/v1.zip">Source code</a></li>
		<li><a href="/u/r/archive/refs/tags/v1.tar.gz">Source code</a></li>
	</ul></div>`

	assets, err := parseExpandedAssets([]byte(body), ripgrepExpandedAssetsURL)
	require.NoError(t, err)
	assert.Empty(t, assets)
}

func TestParseExpandedAssets_MalformedPage_ReturnsErrUnexpectedPage(t *testing.T) {
	body := `<html><body><p>404 Not Found</p></body></html>`

	_, err := parseExpandedAssets([]byte(body), ripgrepExpandedAssetsURL)
	assert.ErrorIs(t, err, ErrUnexpectedPage)
}

func TestParseExpandedAssets_UnparseablePageURL_ReturnsErrUnexpectedPage(t *testing.T) {
	body := `<div><ul><li><a href="/u/r/releases/download/v1/a.zip">a.zip</a></li></ul></div>`

	_, err := parseExpandedAssets([]byte(body), "://not a url")
	assert.ErrorIs(t, err, ErrUnexpectedPage)
}

func TestParseExpandedAssets_MissingHref_SkipsTheItem(t *testing.T) {
	body := `<div><ul>
		<li><span>no link here</span></li>
	</ul></div>`

	assets, err := parseExpandedAssets([]byte(body), ripgrepExpandedAssetsURL)
	require.NoError(t, err)
	assert.Empty(t, assets)
}

func TestParseExpandedAssets_LinkWithoutHrefAttribute_SkipsTheItem(t *testing.T) {
	body := `<div><ul>
		<li><a data-turbo="false">no href attribute</a></li>
	</ul></div>`

	assets, err := parseExpandedAssets([]byte(body), ripgrepExpandedAssetsURL)
	require.NoError(t, err)
	assert.Empty(t, assets)
}

func TestParseExpandedAssets_AlreadyAbsoluteHref_IsKeptAsIs(t *testing.T) {
	body := `<div><ul>
		<li><a href="https://objects.githubusercontent.com/release/1/a.zip">a.zip</a></li>
	</ul></div>`

	assets, err := parseExpandedAssets([]byte(body), ripgrepExpandedAssetsURL)
	require.NoError(t, err)
	require.Len(t, assets, 1)
	assert.Equal(t, "https://objects.githubusercontent.com/release/1/a.zip", assets[0].URL)
}

func TestResolveHref_UnparseableHref_ReturnsItUnchanged(t *testing.T) {
	base, err := url.Parse(ripgrepExpandedAssetsURL)
	require.NoError(t, err)

	got := resolveHref(base, "://bad href")
	assert.Equal(t, "://bad href", got)
}

func TestParseSize_TableDriven(t *testing.T) {
	testCases := []struct {
		name   string
		number string
		unit   string
		want   int64
	}{
		{name: "bytes", number: "109", unit: "Bytes", want: 109},
		{name: "kb", number: "2", unit: "KB", want: 2048},
		{name: "mb", number: "1.5", unit: "MB", want: 1572864},
		{name: "gb", number: "1", unit: "GB", want: 1 << 30},
		{name: "tb", number: "2", unit: "TB", want: 2 << 40},
		{name: "unknown unit", number: "1", unit: "PB", want: 0},
		{name: "unparseable number", number: "not-a-number", unit: "MB", want: 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseSize(tc.number, tc.unit))
		})
	}
}
