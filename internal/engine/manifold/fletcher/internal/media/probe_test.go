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
	if strings.HasSuffix(url, "/"+iconProbePaths()[0]) {
		select {
		case <-o.firstWaitsFor:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return o.stubHost.fetch(ctx, url)
}

func TestProbeIcon_PriorityOrderWinsOverArrivalOrder(t *testing.T) {
	paths := iconProbePaths()
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

func TestAcceptProbedIcon_Cases(
	t *testing.T,
) {
	testCases := []struct {
		name string
		path string
		data []byte
		want bool
	}{
		{name: "svg accepted unconditionally", path: "logo.svg", data: []byte(`<svg><path/></svg>`), want: true},
		{name: "square and large enough png", path: "icon.png", data: encodePNG(t, 128, 128), want: true},
		{name: "too small png", path: "icon.png", data: encodePNG(t, 64, 64)},
		{name: "non square png", path: "icon.png", data: encodePNG(t, 256, 128)},
		{name: "unsniffable data", path: "icon.png", data: []byte("not an image")},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, acceptProbedIcon(tc.path, tc.data))
		})
	}
}
