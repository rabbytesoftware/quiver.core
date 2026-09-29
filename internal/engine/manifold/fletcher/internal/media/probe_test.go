package media

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type orderedHost struct {
	*stubHost
	firstWaitsFor chan struct{}
	lastPath      string
}

func (o *orderedHost) fetch(
	ctx context.Context,
	url string,
) ([]byte, error) {
	if strings.HasSuffix(url, "/"+o.lastPath) {
		defer close(o.firstWaitsFor)
	}
	if strings.HasSuffix(url, "/"+IconProbePaths()[0]) {
		select {
		case <-o.firstWaitsFor:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return o.stubHost.fetch(ctx, url)
}

func TestProbeIcon_PriorityOrderWinsOverArrivalOrder(t *testing.T) {
	paths := IconProbePaths()
	host := &orderedHost{
		stubHost: &stubHost{files: map[string][]byte{
			paths[0]: encodePNG(t, 256, 256),
			paths[1]: encodePNG(t, 256, 256),
			paths[2]: []byte(squareSVG),
		}},
		firstWaitsFor: make(chan struct{}),
		lastPath:      paths[2],
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	icon := ProbeIcon(ctx, host.fetch, host, testNS, testRef)

	assert.Equal(t, "https://raw.githubusercontent.com/owner/repo/v1.0.0/"+paths[0], icon)
}

func TestProbeIcon_NoAcceptedCandidate(t *testing.T) {
	testCases := []struct {
		name string
		host *stubHost
	}{
		{name: "nothing published", host: &stubHost{}},
		{
			name: "only rejected images",
			host: &stubHost{files: map[string][]byte{
				"src-tauri/icons/icon.png": encodePNG(t, 64, 64),
				"build/icon.png":           encodePNG(t, 256, 128),
			}},
		},
		{
			name: "host without file urls",
			host: &stubHost{
				noTemplates: true,
				files:       map[string][]byte{"logo.svg": []byte(squareSVG)},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, ProbeIcon(context.Background(), tc.host.fetch, tc.host, testNS, testRef))
		})
	}
}

func TestIconProbePaths_MatchesSpecOrder(
	t *testing.T,
) {
	want := []string{
		"src-tauri/icons/icon.png",
		"build/icon.png",
		"logo.svg",
	}

	assert.Equal(t, want, IconProbePaths())
}

func TestIsRejectedIconPath_Classification(
	t *testing.T,
) {
	testCases := []struct {
		name string
		path string
		want bool
	}{
		{name: "ico rejected", path: "icon.ico", want: true},
		{name: "icns rejected", path: "icon.icns", want: true},
		{name: "favicon rejected", path: "favicon.png", want: true},
		{name: "uppercase favicon rejected", path: "FAVICON.PNG", want: true},
		{name: "plain png accepted", path: "logo.png", want: false},
		{name: "svg accepted", path: "logo.svg", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRejectedIconPath(tc.path))
		})
	}
}

func TestAcceptProbedIcon_SVGAcceptedUnconditionally(
	t *testing.T,
) {
	accepted := acceptProbedIcon("logo.svg", []byte(`<svg><path/></svg>`))

	assert.True(t, accepted)
}

func TestAcceptProbedIcon_RejectsFavicon(
	t *testing.T,
) {
	accepted := acceptProbedIcon("favicon.png", encodePNG(t, 256, 256))

	assert.False(t, accepted)
}

func TestAcceptProbedIcon_RejectsIco(
	t *testing.T,
) {
	accepted := acceptProbedIcon("icon.ico", []byte{0, 0, 1, 0})

	assert.False(t, accepted)
}

func TestAcceptProbedIcon_PNGSquareAndLargeEnoughAccepted(
	t *testing.T,
) {
	accepted := acceptProbedIcon("icon.png", encodePNG(t, 128, 128))

	assert.True(t, accepted)
}

func TestAcceptProbedIcon_PNGTooSmallRejected(
	t *testing.T,
) {
	accepted := acceptProbedIcon("icon.png", encodePNG(t, 64, 64))

	assert.False(t, accepted)
}

func TestAcceptProbedIcon_PNGNonSquareRejected(
	t *testing.T,
) {
	accepted := acceptProbedIcon("icon.png", encodePNG(t, 256, 128))

	assert.False(t, accepted)
}

func TestAcceptProbedIcon_UnsniffableDataRejected(
	t *testing.T,
) {
	accepted := acceptProbedIcon("icon.png", []byte("not an image"))

	assert.False(t, accepted)
}
