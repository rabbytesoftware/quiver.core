package readme

import (
	"fmt"
	"regexp"
	"strings"
)

const FilePlaceholder = "{file}"

type RawBase struct {
	Raw  string
	Blob string
}

var mdLinkOrImagePattern = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)\s]+)(\s+"[^"]*")?\)`)

var htmlImgSrcPattern = regexp.MustCompile(`(<img\s+[^>]*?src=")([^"]+)(")`)

func IsRelativePath(
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

func CleanRelativePath(
	path string,
) string {
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")
	return path
}

func fileURL(
	template string,
	path string,
) string {
	if template == "" {
		return path
	}
	return strings.ReplaceAll(template, FilePlaceholder, CleanRelativePath(path))
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
		if !IsRelativePath(url) {
			return match
		}
		if isImage {
			return fmt.Sprintf("![%s](%s)%s", alt, fileURL(base.Raw, url), title)
		}
		return fmt.Sprintf("[%s](%s)%s", alt, fileURL(base.Blob, url), title)
	})

	line = htmlImgSrcPattern.ReplaceAllStringFunc(line, func(match string) string {
		sub := htmlImgSrcPattern.FindStringSubmatch(match)
		if !IsRelativePath(sub[2]) {
			return match
		}
		return sub[1] + fileURL(base.Raw, sub[2]) + sub[3]
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
