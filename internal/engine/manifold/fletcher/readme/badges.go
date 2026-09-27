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

func classifyImageMatch(
	src string,
	sawBadgeHost *bool,
	sawNonBadge *bool,
) {
	if IsBadgeSrc(src) {
		*sawBadgeHost = true
		return
	}
	*sawNonBadge = true
}

func stripImageTokens(
	line string,
) (string, bool, bool, bool) {
	remainder := line
	sawImage := false
	sawBadgeHost := false
	sawNonBadge := false

	remainder = linkedImagePattern.ReplaceAllStringFunc(remainder, func(match string) string {
		sub := linkedImagePattern.FindStringSubmatch(match)
		sawImage = true
		classifyImageMatch(sub[1], &sawBadgeHost, &sawNonBadge)
		return ""
	})

	remainder = bareImagePattern.ReplaceAllStringFunc(remainder, func(match string) string {
		sub := bareImagePattern.FindStringSubmatch(match)
		sawImage = true
		classifyImageMatch(sub[1], &sawBadgeHost, &sawNonBadge)
		return ""
	})

	return remainder, sawImage, sawBadgeHost, sawNonBadge
}

func stripHTMLBadgeTokens(
	line string,
) (string, bool, bool, bool) {
	remainder := line
	sawImage := false
	sawBadgeHost := false
	sawNonBadge := false

	remainder = htmlImgTagPattern.ReplaceAllStringFunc(remainder, func(match string) string {
		sub := htmlImgTagPattern.FindStringSubmatch(match)
		sawImage = true
		classifyImageMatch(sub[1], &sawBadgeHost, &sawNonBadge)
		return ""
	})
	remainder = htmlAnchorPattern.ReplaceAllString(remainder, "")
	remainder = htmlBrPattern.ReplaceAllString(remainder, "")

	return remainder, sawImage, sawBadgeHost, sawNonBadge
}

func isMarkdownBadgeOnlyLine(
	line string,
) bool {
	remainder, sawImage, sawBadgeHost, sawNonBadge := stripImageTokens(line)
	if !sawImage || !sawBadgeHost || sawNonBadge {
		return false
	}
	return strings.TrimSpace(remainder) == ""
}

func isHTMLBadgeOnlyLine(
	line string,
) bool {
	remainder, sawImage, sawBadgeHost, sawNonBadge := stripHTMLBadgeTokens(line)
	if !sawImage || !sawBadgeHost || sawNonBadge {
		return false
	}
	return strings.TrimSpace(remainder) == ""
}

func isBadgeOnlyLine(
	line string,
) bool {
	if isMarkdownBadgeOnlyLine(line) {
		return true
	}
	return isHTMLBadgeOnlyLine(line)
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

func markRemoved(
	remove []bool,
	from int,
	to int,
) {
	for k := from; k <= to; k++ {
		remove[k] = true
	}
}

func filterOut(
	lines []string,
	remove []bool,
) []string {
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if remove[i] {
			continue
		}
		out = append(out, line)
	}
	return out
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
		markRemoved(remove, i, end)
		i = end + 1
	}
	return filterOut(lines, remove)
}
