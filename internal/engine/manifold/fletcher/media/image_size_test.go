package media

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodePNG(
	t *testing.T,
	width int,
	height int,
) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func encodeGIF(
	t *testing.T,
	width int,
	height int,
) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, width, height), color.Palette{color.White, color.Black})
	var buf bytes.Buffer
	require.NoError(t, gif.Encode(&buf, img, nil))
	return buf.Bytes()
}

func encodeJPEG(
	t *testing.T,
	width int,
	height int,
) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	return buf.Bytes()
}

func TestSniff_DetectsPNGDimensions(
	t *testing.T,
) {
	data := encodePNG(t, 256, 256)

	dim, ok := Sniff(data)

	require.True(t, ok)
	assert.Equal(t, Dimensions{Width: 256, Height: 256}, dim)
}

func TestSniff_DetectsGIFDimensions(
	t *testing.T,
) {
	data := encodeGIF(t, 64, 32)

	dim, ok := Sniff(data)

	require.True(t, ok)
	assert.Equal(t, Dimensions{Width: 64, Height: 32}, dim)
}

func TestSniff_DetectsJPEGDimensions(
	t *testing.T,
) {
	data := encodeJPEG(t, 128, 96)

	dim, ok := Sniff(data)

	require.True(t, ok)
	assert.Equal(t, Dimensions{Width: 128, Height: 96}, dim)
}

func TestSniff_DetectsSVGDimensionsFromWidthHeight(
	t *testing.T,
) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="150" height="150"><path/></svg>`)

	dim, ok := Sniff(svg)

	require.True(t, ok)
	assert.Equal(t, Dimensions{Width: 150, Height: 150}, dim)
}

func TestSniff_DetectsSVGDimensionsFromViewBox(
	t *testing.T,
) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100"><path/></svg>`)

	dim, ok := Sniff(svg)

	require.True(t, ok)
	assert.Equal(t, Dimensions{Width: 200, Height: 100}, dim)
}

func TestSniff_SVGPrefersWidthHeightOverViewBox(
	t *testing.T,
) {
	svg := []byte(`<svg width="10" height="10" viewBox="0 0 999 999"><path/></svg>`)

	dim, ok := Sniff(svg)

	require.True(t, ok)
	assert.Equal(t, Dimensions{Width: 10, Height: 10}, dim)
}

func TestSniff_SVGWithZeroDimensionsFails(
	t *testing.T,
) {
	svg := []byte(`<svg width="0" height="0"><path/></svg>`)

	_, ok := Sniff(svg)

	assert.False(t, ok)
}

func TestSniff_SVGWithNoDimensionsFails(
	t *testing.T,
) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path/></svg>`)

	_, ok := Sniff(svg)

	assert.False(t, ok)
}

func TestSniff_EmptyDataFails(
	t *testing.T,
) {
	_, ok := Sniff(nil)

	assert.False(t, ok)
}

func TestSniff_GarbageDataFails(
	t *testing.T,
) {
	_, ok := Sniff([]byte("not an image at all"))

	assert.False(t, ok)
}

func TestIsSVG_DetectsTagWithinFirstBytes(
	t *testing.T,
) {
	testCases := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "svg tag present", data: []byte(`<?xml version="1.0"?><svg></svg>`), want: true},
		{name: "png header", data: []byte("\x89PNG\r\n\x1a\n"), want: false},
		{name: "empty", data: []byte(""), want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isSVG(tc.data))
		})
	}
}
