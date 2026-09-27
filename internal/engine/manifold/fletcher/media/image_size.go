package media

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"regexp"
	"strconv"
)

type Dimensions struct {
	Width  int
	Height int
}

var svgViewBoxPattern = regexp.MustCompile(`viewBox\s*=\s*"[^"]*?\s+([0-9.]+)\s+([0-9.]+)\s*"`)

var svgWidthPattern = regexp.MustCompile(`\bwidth\s*=\s*"([0-9.]+)(?:px)?"`)

var svgHeightPattern = regexp.MustCompile(`\bheight\s*=\s*"([0-9.]+)(?:px)?"`)

func isSVG(
	data []byte,
) bool {
	return bytes.Contains(data[:min(len(data), 512)], []byte("<svg"))
}

func parsedDimensions(
	rawWidth []byte,
	rawHeight []byte,
) (Dimensions, bool) {
	w, errW := strconv.ParseFloat(string(rawWidth), 64)
	h, errH := strconv.ParseFloat(string(rawHeight), 64)
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return Dimensions{}, false
	}
	return Dimensions{Width: int(w), Height: int(h)}, true
}

func sniffSVGWidthHeight(
	data []byte,
) (Dimensions, bool) {
	width := svgWidthPattern.FindSubmatch(data)
	height := svgHeightPattern.FindSubmatch(data)
	if width == nil || height == nil {
		return Dimensions{}, false
	}
	return parsedDimensions(width[1], height[1])
}

func sniffSVGViewBox(
	data []byte,
) (Dimensions, bool) {
	match := svgViewBoxPattern.FindSubmatch(data)
	if match == nil {
		return Dimensions{}, false
	}
	return parsedDimensions(match[1], match[2])
}

func sniffSVG(
	data []byte,
) (Dimensions, bool) {
	if dim, ok := sniffSVGWidthHeight(data); ok {
		return dim, true
	}
	return sniffSVGViewBox(data)
}

func Sniff(
	data []byte,
) (Dimensions, bool) {
	if len(data) == 0 {
		return Dimensions{}, false
	}
	if isSVG(data) {
		return sniffSVG(data)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Dimensions{}, false
	}
	return Dimensions{Width: cfg.Width, Height: cfg.Height}, true
}
