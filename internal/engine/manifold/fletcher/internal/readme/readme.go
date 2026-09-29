package readme

import (
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func splitLines(
	raw []byte,
) []string {
	return strings.Split(string(raw), "\n")
}

func collapseBlankLines(
	lines []string,
) []string {
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		isBlank := strings.TrimSpace(line) == ""
		if isBlank && blank {
			continue
		}
		out = append(out, line)
		blank = isBlank
	}
	return out
}

func truncate(
	text string,
	limit int,
) string {
	if len(text) <= limit {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

func Transform(
	raw []byte,
	base RawBase,
) string {
	lines := splitLines(raw)
	lines = neutralizeFences(lines)
	lines = stripLeadingBlock(lines)
	lines = stripHTMLBadgeBlocks(lines)
	lines = stripBadgeLines(lines)
	lines = stripInstallSections(lines)
	lines = rewriteLinks(lines, base)
	lines = collapseBlankLines(lines)

	text := strings.TrimSpace(strings.Join(lines, "\n"))
	return truncate(text, domain.MaxReadmeLength)
}
