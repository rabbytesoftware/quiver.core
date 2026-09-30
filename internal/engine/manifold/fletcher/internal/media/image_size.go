package media

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

type dimensions struct {
	Width  float64
	Height float64
	Vector bool
}

func sniff(
	data []byte,
) (dimensions, bool) {
	if len(data) == 0 {
		return dimensions{}, false
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		return dimensions{Width: float64(cfg.Width), Height: float64(cfg.Height)}, true
	}
	return sniffSVG(data)
}
