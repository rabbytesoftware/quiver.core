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

func TestSniff_Cases(
	t *testing.T,
) {
	testCases := []struct {
		name   string
		data   []byte
		want   dimensions
		wantOK bool
	}{
		{name: "png", data: encodePNG(t, 256, 256), want: dimensions{Width: 256, Height: 256}, wantOK: true},
		{name: "gif", data: encodeGIF(t, 64, 32), want: dimensions{Width: 64, Height: 32}, wantOK: true},
		{name: "jpeg", data: encodeJPEG(t, 128, 96), want: dimensions{Width: 128, Height: 96}, wantOK: true},
		{name: "svg width and height", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="150" height="150"><path/></svg>`), want: dimensions{Width: 150, Height: 150}, wantOK: true},
		{name: "svg view box", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100"><path/></svg>`), want: dimensions{Width: 200, Height: 100}, wantOK: true},
		{name: "svg prefers width and height", data: []byte(`<svg width="10" height="10" viewBox="0 0 999 999"><path/></svg>`), want: dimensions{Width: 10, Height: 10}, wantOK: true},
		{name: "svg zero dimensions", data: []byte(`<svg width="0" height="0"><path/></svg>`)},
		{name: "svg without dimensions", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path/></svg>`)},
		{name: "empty", data: nil},
		{name: "garbage", data: []byte("not an image at all")},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := sniff(tc.data)

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}
