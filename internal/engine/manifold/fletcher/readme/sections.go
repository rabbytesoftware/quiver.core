package readme

import (
	"regexp"
	"strings"
)

var atxHeadingPattern = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)

var installHeadingPattern = regexp.MustCompile(`(?i)^(install|installation|download|downloads)\b`)

var setextUnderlinePattern = regexp.MustCompile(`^(=+|-+)\s*$`)

var hrPattern = regexp.MustCompile(`^(?:-{3,}|\*{3,}|_{3,})\s*$`)

func isFenceDelim(
	line string,
) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}

func fenceStates(
	lines []string,
) []bool {
	states := make([]bool, len(lines))
	inFence := false
	for i, line := range lines {
		if isFenceDelim(line) {
			states[i] = true
			inFence = !inFence
			continue
		}
		states[i] = inFence
	}
	return states
}

func neutralizeFences(
	lines []string,
) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "```") {
			out[i] = line
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		rest := trimmed
		count := 0
		for count < len(rest) && rest[count] == '`' {
			count++
		}
		out[i] = indent + strings.Repeat("~", count) + rest[count:]
	}
	return out
}

func isLeadingVisualLine(
	line string,
) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true
	}
	if hrPattern.MatchString(trimmed) {
		return true
	}
	return isBadgeOnlyLine(line)
}

func stripLeadingBlock(
	lines []string,
) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start >= len(lines) {
		return lines
	}

	consumed, found := leadingHeadingEnd(lines, start)
	if !found {
		return lines
	}

	for consumed < len(lines) && isLeadingVisualLine(lines[consumed]) {
		consumed++
	}

	out := make([]string, 0, len(lines)-consumed+start)
	out = append(out, lines[:start]...)
	out = append(out, lines[consumed:]...)
	return out
}

func leadingHeadingEnd(
	lines []string,
	start int,
) (int, bool) {
	if match := atxHeadingPattern.FindStringSubmatch(lines[start]); match != nil && len(match[1]) == 1 {
		return start + 1, true
	}
	if start+1 >= len(lines) || !setextUnderlinePattern.MatchString(lines[start+1]) {
		return 0, false
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[start+1]), "-") {
		return 0, false
	}
	return start + 2, true
}

func stripInstallSections(
	lines []string,
) []string {
	inFence := fenceStates(lines)
	out := make([]string, 0, len(lines))
	skipLevel := 0
	for i, line := range lines {
		var skip bool
		skipLevel, skip = installSectionState(line, inFence[i], skipLevel)
		if skip {
			continue
		}
		out = append(out, line)
	}
	return out
}

func installSectionState(
	line string,
	inFence bool,
	skipLevel int,
) (int, bool) {
	match := atxHeadingPattern.FindStringSubmatch(line)
	if inFence || match == nil {
		return skipLevel, skipLevel > 0
	}
	level := len(match[1])
	if skipLevel > 0 && level <= skipLevel {
		skipLevel = 0
	}
	if skipLevel == 0 && installHeadingPattern.MatchString(strings.TrimSpace(match[2])) {
		return level, true
	}
	return skipLevel, skipLevel > 0
}

func NeutralizeFences(
	text string,
) string {
	return strings.Join(neutralizeFences(strings.Split(text, "\n")), "\n")
}
