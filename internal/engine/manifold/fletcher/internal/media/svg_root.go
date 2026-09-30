package media

import (
	"bytes"
	"encoding/xml"
	"strconv"
	"strings"
)

const svgHeadBytes = 4096

var svgLengthUnits = []string{"px", "pt", "em", "mm", "cm", "in"}

func sniffSVG(
	data []byte,
) (dimensions, bool) {
	root, ok := svgRoot(data)
	if !ok {
		return dimensions{}, false
	}
	if dim, ok := svgSize(root); ok {
		return dim, true
	}
	return svgViewBox(root)
}

func svgRoot(
	data []byte,
) ([]xml.Attr, bool) {
	head := bytes.TrimPrefix(data[:min(len(data), svgHeadBytes)], []byte("\xef\xbb\xbf"))
	dec := xml.NewDecoder(bytes.NewReader(head))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		return start.Attr, start.Name.Local == "svg"
	}
}

func svgAttr(
	attrs []xml.Attr,
	name string,
) string {
	for _, attr := range attrs {
		if attr.Name.Space == "" && attr.Name.Local == name {
			return strings.TrimSpace(attr.Value)
		}
	}
	return ""
}

func svgSize(
	attrs []xml.Attr,
) (dimensions, bool) {
	width, okW := svgLength(svgAttr(attrs, "width"))
	height, okH := svgLength(svgAttr(attrs, "height"))
	if !okW || !okH {
		return dimensions{}, false
	}
	return dimensions{Width: width, Height: height, Vector: true}, true
}

func svgLength(
	raw string,
) (float64, bool) {
	for _, unit := range svgLengthUnits {
		raw = strings.TrimSuffix(raw, unit)
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return value, err == nil && value > 0
}

func svgViewBox(
	attrs []xml.Attr,
) (dimensions, bool) {
	fields := strings.FieldsFunc(svgAttr(attrs, "viewBox"), func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) != 4 {
		return dimensions{}, false
	}
	width, errW := strconv.ParseFloat(fields[2], 64)
	height, errH := strconv.ParseFloat(fields[3], 64)
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return dimensions{}, false
	}
	return dimensions{Width: width, Height: height, Vector: true}, true
}
