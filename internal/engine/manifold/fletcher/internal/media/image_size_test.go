package media

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
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
		{name: "svg width and height", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="150" height="150"><path/></svg>`), want: dimensions{Width: 150, Height: 150, Vector: true}, wantOK: true},
		{name: "svg px units", data: []byte(`<svg width="64px" height="32px"></svg>`), want: dimensions{Width: 64, Height: 32, Vector: true}, wantOK: true},
		{name: "svg view box", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100"><path/></svg>`), want: dimensions{Width: 200, Height: 100, Vector: true}, wantOK: true},
		{name: "svg view box with commas", data: []byte(`<svg viewBox="0,0,24,24"></svg>`), want: dimensions{Width: 24, Height: 24, Vector: true}, wantOK: true},
		{name: "svg prefers width and height", data: []byte(`<svg width="10" height="10" viewBox="0 0 999 999"><path/></svg>`), want: dimensions{Width: 10, Height: 10, Vector: true}, wantOK: true},
		{name: "svg percentages fall to view box", data: []byte(`<svg width="100%" height="100%" viewBox="0 0 48 48"></svg>`), want: dimensions{Width: 48, Height: 48, Vector: true}, wantOK: true},
		{name: "svg one non numeric side falls to view box", data: []byte(`<svg width="auto" height="20" viewBox="0 0 30 30"></svg>`), want: dimensions{Width: 30, Height: 30, Vector: true}, wantOK: true},
		{name: "svg with prolog comment and doctype", data: []byte("\xef\xbb\xbf<?xml version=\"1.0\"?>\n<!-- made by hand -->\n<!DOCTYPE svg PUBLIC \"-//W3C//DTD SVG 1.1//EN\" \"http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd\">\n<svg width=\"32\" height=\"32\"></svg>"), want: dimensions{Width: 32, Height: 32, Vector: true}, wantOK: true},
		{name: "svg root after a long comment", data: []byte("<!--" + strings.Repeat("x", 3000) + "--><svg width=\"9\" height=\"9\"></svg>"), want: dimensions{Width: 9, Height: 9, Vector: true}, wantOK: true},
		{name: "svg root past the head window", data: []byte("<!--" + strings.Repeat("x", 5000) + "--><svg width=\"9\" height=\"9\"></svg>")},
		{name: "child attributes are not the root", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="500" height="500"/></svg>`)},
		{name: "stroke width is not width", data: []byte(`<svg stroke-width="5" viewBox="0 0 16 16"></svg>`), want: dimensions{Width: 16, Height: 16, Vector: true}, wantOK: true},
		{name: "svg zero dimensions", data: []byte(`<svg width="0" height="0"><path/></svg>`)},
		{name: "svg malformed view box", data: []byte(`<svg viewBox="0 0 20"></svg>`)},
		{name: "svg non numeric view box", data: []byte(`<svg viewBox="0 0 a b"></svg>`)},
		{name: "svg without dimensions", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path/></svg>`)},
		{name: "html mentioning svg", data: []byte(`<html><body><svg width="20" height="20"></svg></body></html>`)},
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
