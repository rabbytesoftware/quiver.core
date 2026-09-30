package media

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProbeIcon_Cases(
	t *testing.T,
) {
	big := encodePNG(t, 256, 256)
	testCases := []struct {
		name   string
		tagged map[string][]byte
		branch map[string][]byte
		want   string
	}{
		{name: "tauri icon first", tagged: map[string][]byte{"src-tauri/icons/icon.png": big, "assets/icon.png": big}, want: githubRawPrefix + "src-tauri/icons/icon.png"},
		{name: "assets logo", tagged: map[string][]byte{"assets/logo.png": big}, want: githubRawPrefix + "assets/logo.png"},
		{name: "github folder app icon", tagged: map[string][]byte{".github/app-icon.svg": []byte(squareSVG)}, want: githubRawPrefix + ".github/app-icon.svg"},
		{name: "root png beats root svg", tagged: map[string][]byte{"icon.png": big, "icon.svg": []byte(squareSVG)}, want: githubRawPrefix + "icon.png"},
		{name: "too small png falls to the next", tagged: map[string][]byte{"icon.png": encodePNG(t, 64, 64), "logo.png": big}, want: githubRawPrefix + "logo.png"},
		{name: "non square png is skipped", tagged: map[string][]byte{"logo.png": encodePNG(t, 400, 128)}},
		{name: "wide svg is skipped", tagged: map[string][]byte{"logo.svg": []byte(`<svg width="300" height="60"></svg>`)}},
		{name: "html masquerading as svg is skipped", tagged: map[string][]byte{"logo.svg": []byte("<html>404</html>")}},
		{name: "svg by view box only", tagged: map[string][]byte{"docs/logo.svg": []byte(`<svg width="100%" height="100%" viewBox="0 0 24 24"></svg>`)}, want: githubRawPrefix + "docs/logo.svg"},
		{name: "default branch backs up the tag", branch: map[string][]byte{"assets/icon.png": big}, want: githubMainPrefix + "assets/icon.png"},
		{name: "tag wins over the default branch", tagged: map[string][]byte{"build/logo.png": big}, branch: map[string][]byte{"icon.png": big}, want: githubRawPrefix + "build/logo.png"},
		{name: "nothing found"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host := &stubHost{tagged: tc.tagged, branch: tc.branch}

			got := ProbeIcon(context.Background(), host.fetch, host, testNS, testRef)

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestProbeIcon_HostWithoutRawURLsProbesNothing(
	t *testing.T,
) {
	host := &stubHost{tagged: map[string][]byte{"logo.svg": []byte(squareSVG)}, noTemplates: true}

	got := ProbeIcon(context.Background(), host.fetch, host, testNS, testRef)

	assert.Empty(t, got)
}

func TestIconProbePaths_AreUniqueAndCoverTheTable(
	t *testing.T,
) {
	paths := iconProbePaths()

	seen := map[string]bool{}
	for _, p := range paths {
		assert.False(t, seen[p], p)
		assert.False(t, strings.HasPrefix(p, "/"), p)
		seen[p] = true
	}
	assert.Contains(t, paths, "icon.png")
	assert.Contains(t, paths, "assets/logo.svg")
	assert.Contains(t, paths, "src-tauri/icons/icon.png")
	assert.Equal(t, "src-tauri/icons/icon.png", paths[0])
}

func TestIsSquareDim_Tolerance(
	t *testing.T,
) {
	assert.True(t, isSquareDim(dimensions{Width: 100, Height: 101}))
	assert.False(t, isSquareDim(dimensions{Width: 100, Height: 110}))
}

func TestProbeIcon_AHitStopsTheLowerRankedFetches(
	t *testing.T,
) {
	host := &stubHost{tagged: map[string][]byte{"src-tauri/icons/icon.png": encodePNG(t, 256, 256)}}
	var fetched atomic.Int64
	fetch := func(ctx context.Context, url string) ([]byte, error) {
		fetched.Add(1)
		return host.fetch(ctx, url)
	}

	got := ProbeIcon(context.Background(), fetch, host, testNS, testRef)

	assert.Equal(t, githubRawPrefix+"src-tauri/icons/icon.png", got)
	assert.Less(t, fetched.Load(), int64(len(iconProbePaths())))
}

func TestProbeIcon_RefEqualToADefaultBranchIsProbedOnce(
	t *testing.T,
) {
	host := &stubHost{}
	var fetched atomic.Int64
	fetch := func(_ context.Context, _ string) ([]byte, error) {
		fetched.Add(1)
		return nil, errMissing
	}

	got := ProbeIcon(context.Background(), fetch, host, testNS, testBranch)

	assert.Empty(t, got)
	assert.Equal(t, int64(len(iconProbePaths())), fetched.Load())
}
