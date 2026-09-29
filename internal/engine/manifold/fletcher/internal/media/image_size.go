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

type dimensions struct {
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
) (dimensions, bool) {
	w, errW := strconv.ParseFloat(string(rawWidth), 64)
	h, errH := strconv.ParseFloat(string(rawHeight), 64)
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return dimensions{}, false
	}
	return dimensions{Width: int(w), Height: int(h)}, true
}

func sniffSVGWidthHeight(
	data []byte,
) (dimensions, bool) {
	width := svgWidthPattern.FindSubmatch(data)
	height := svgHeightPattern.FindSubmatch(data)
	if width == nil || height == nil {
		return dimensions{}, false
	}
	return parsedDimensions(width[1], height[1])
}

func sniffSVGViewBox(
	data []byte,
) (dimensions, bool) {
	match := svgViewBoxPattern.FindSubmatch(data)
	if match == nil {
		return dimensions{}, false
	}
	return parsedDimensions(match[1], match[2])
}

func sniffSVG(
	data []byte,
) (dimensions, bool) {
	if dim, ok := sniffSVGWidthHeight(data); ok {
		return dim, true
	}
	return sniffSVGViewBox(data)
}

func sniff(
	data []byte,
) (dimensions, bool) {
	if len(data) == 0 {
		return dimensions{}, false
	}
	if isSVG(data) {
		return sniffSVG(data)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return dimensions{}, false
	}
	return dimensions{Width: cfg.Width, Height: cfg.Height}, true
}
