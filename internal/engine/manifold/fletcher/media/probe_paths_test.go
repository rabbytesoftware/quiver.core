package media

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIconProbePaths_MatchesSpecOrder(
	t *testing.T,
) {
	want := []string{
		"src-tauri/icons/icon.png",
		"build/icon.png",
		"resources/icon.png",
		"assets/icon.png",
		"assets/logo.svg",
		"logo.svg",
		".github/logo.svg",
		".github/logo.png",
		"docs/logo.svg",
		"icon.png",
		"logo.png",
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
