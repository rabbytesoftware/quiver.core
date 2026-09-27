package readme

import (
	"fmt"
	"regexp"
	"strings"
)

type RawBase struct {
	Owner string
	Repo  string
	Ref   string
}

type Image struct {
	Src  string
	Line int
}

var mdLinkOrImagePattern = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)\s]+)(\s+"[^"]*")?\)`)

var htmlImgSrcPattern = regexp.MustCompile(`(<img\s+[^>]*?src=")([^"]+)(")`)

func isRelativePath(
	url string,
) bool {
	if strings.Contains(url, "://") {
		return false
	}
	if strings.HasPrefix(url, "//") {
		return false
	}
	if strings.HasPrefix(url, "#") {
		return false
	}
	if strings.HasPrefix(url, "mailto:") || strings.HasPrefix(url, "tel:") {
		return false
	}
	return true
}

func cleanRelativePath(
	path string,
) string {
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")
	return path
}

func RawImageURL(
	base RawBase,
	path string,
) string {
	return fmt.Sprintf(
		"https://raw.githubusercontent.com/%s/%s/%s/%s",
		base.Owner,
		base.Repo,
		base.Ref,
		cleanRelativePath(path),
	)
}

func BlobLinkURL(
	base RawBase,
	path string,
) string {
	return fmt.Sprintf(
		"https://github.com/%s/%s/blob/%s/%s",
		base.Owner,
		base.Repo,
		base.Ref,
		cleanRelativePath(path),
	)
}

func rewriteLine(
	line string,
	base RawBase,
) string {
	line = mdLinkOrImagePattern.ReplaceAllStringFunc(line, func(match string) string {
		sub := mdLinkOrImagePattern.FindStringSubmatch(match)
		isImage := sub[1] == "!"
		alt := sub[2]
		url := sub[3]
		title := sub[4]
		if !isRelativePath(url) {
			return match
		}
		if isImage {
			return fmt.Sprintf("![%s](%s)%s", alt, RawImageURL(base, url), title)
		}
		return fmt.Sprintf("[%s](%s)%s", alt, BlobLinkURL(base, url), title)
	})

	line = htmlImgSrcPattern.ReplaceAllStringFunc(line, func(match string) string {
		sub := htmlImgSrcPattern.FindStringSubmatch(match)
		if !isRelativePath(sub[2]) {
			return match
		}
		return sub[1] + RawImageURL(base, sub[2]) + sub[3]
	})

	return line
}

func rewriteLinks(
	lines []string,
	base RawBase,
) []string {
	inFence := fenceStates(lines)
	out := make([]string, len(lines))
	for i, line := range lines {
		if inFence[i] {
			out[i] = line
			continue
		}
		out[i] = rewriteLine(line, base)
	}
	return out
}

func imagesInLine(
	line string,
) []string {
	var srcs []string
	for _, sub := range mdLinkOrImagePattern.FindAllStringSubmatch(line, -1) {
		if sub[1] == "!" {
			srcs = append(srcs, sub[3])
		}
	}
	for _, sub := range htmlImgSrcPattern.FindAllStringSubmatch(line, -1) {
		srcs = append(srcs, sub[2])
	}
	return srcs
}

func Images(
	raw []byte,
) []Image {
	lines := neutralizeFences(splitLines(raw))
	inFence := fenceStates(lines)
	var images []Image
	for i, line := range lines {
		if inFence[i] {
			continue
		}
		for _, src := range imagesInLine(line) {
			images = append(images, Image{Src: src, Line: i})
		}
	}
	return images
}
