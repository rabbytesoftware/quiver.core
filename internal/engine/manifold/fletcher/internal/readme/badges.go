package readme

import (
	"regexp"
	"strings"
)

var linkedImagePattern = regexp.MustCompile(`\[!\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)\]\([^)\s]+(?:\s+"[^"]*")?\)`)

var bareImagePattern = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)

var htmlImgTagPattern = regexp.MustCompile(`<img\s+[^>]*?src="([^"]+)"[^>]*/?>`)

var htmlAnchorPattern = regexp.MustCompile(`</?a(?:\s+[^>]*)?>`)

var htmlBrPattern = regexp.MustCompile(`<br\s*/?>`)

var htmlOpenBlockPattern = regexp.MustCompile(`(?i)^<(p|div)(?:\s[^>]*)?>$`)

func badgeHosts() []string {
	return []string{
		"shields.io",
		"badge.fury.io",
		"travis-ci.org",
		"travis-ci.com",
		"circleci.com",
		"coveralls.io",
		"codecov.io",
		"opencollective.com",
		"badgen.net",
		"snyk.io",
		"repology.org",
		"deepsource.io",
		"codeclimate.com",
		"sonarcloud.io",
		"github.com/sponsors",
		"badge.svg",
		"workflow/badge",
	}
}

func IsBadgeSrc(
	src string,
) bool {
	lower := strings.ToLower(src)
	for _, host := range badgeHosts() {
		if strings.Contains(lower, host) {
			return true
		}
	}
	return strings.Contains(lower, "badge")
}

func badgeOnly(
	line string,
	images []*regexp.Regexp,
	noise ...*regexp.Regexp,
) bool {
	remainder := line
	sawBadge := false
	sawOther := false
	for _, pattern := range images {
		remainder = pattern.ReplaceAllStringFunc(remainder, func(match string) string {
			if IsBadgeSrc(pattern.FindStringSubmatch(match)[1]) {
				sawBadge = true
			} else {
				sawOther = true
			}
			return ""
		})
	}
	if !sawBadge || sawOther {
		return false
	}
	for _, pattern := range noise {
		remainder = pattern.ReplaceAllString(remainder, "")
	}
	return strings.TrimSpace(remainder) == ""
}

func isHTMLBadgeOnlyLine(
	line string,
) bool {
	return badgeOnly(line, []*regexp.Regexp{htmlImgTagPattern}, htmlAnchorPattern, htmlBrPattern)
}

func isBadgeOnlyLine(
	line string,
) bool {
	return badgeOnly(line, []*regexp.Regexp{linkedImagePattern, bareImagePattern}) || isHTMLBadgeOnlyLine(line)
}

func stripBadgeLines(
	lines []string,
) []string {
	inFence := fenceStates(lines)
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if !inFence[i] && isBadgeOnlyLine(line) {
			continue
		}
		out = append(out, line)
	}
	return out
}

func isBlockFillerLine(
	line string,
) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}
	return strings.TrimSpace(htmlBrPattern.ReplaceAllString(trimmed, "")) == ""
}

func qualifiesForBadgeBlock(
	line string,
) bool {
	if isBlockFillerLine(line) {
		return true
	}
	return isHTMLBadgeOnlyLine(line)
}

func htmlBlockTag(
	line string,
) (string, bool) {
	match := htmlOpenBlockPattern.FindStringSubmatch(strings.TrimSpace(line))
	if match == nil {
		return "", false
	}
	return strings.ToLower(match[1]), true
}

func findBadgeBlockEnd(
	lines []string,
	start int,
	tag string,
) (int, bool) {
	closeLine := "</" + tag + ">"
	for j := start + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == closeLine {
			return j, true
		}
		if !qualifiesForBadgeBlock(lines[j]) {
			return 0, false
		}
	}
	return 0, false
}

func stripHTMLBadgeBlocks(
	lines []string,
) []string {
	inFence := fenceStates(lines)
	remove := make([]bool, len(lines))
	i := 0
	for i < len(lines) {
		tag, isOpen := htmlBlockTag(lines[i])
		if inFence[i] || !isOpen {
			i++
			continue
		}
		end, ok := findBadgeBlockEnd(lines, i, tag)
		if !ok {
			i++
			continue
		}
		for k := i; k <= end; k++ {
			remove[k] = true
		}
		i = end + 1
	}
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if !remove[i] {
			out = append(out, line)
		}
	}
	return out
}
