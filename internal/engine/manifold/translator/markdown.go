package translator

import "strings"

// extractArrowCodeblock extracts the content of the first ```arrow fenced block
// from markdown input. Returns the YAML bytes and true if found, nil and false otherwise.
func extractArrowCodeblock(data []byte) ([]byte, bool) {
	return extractFencedCodeblock(data, "```arrow")
}

// extractCollectionCodeblock extracts the content of the first ```collection fenced block
// from markdown input. Returns the YAML bytes and true if found, nil and false otherwise.
func extractCollectionCodeblock(data []byte) ([]byte, bool) {
	return extractFencedCodeblock(data, "```collection")
}

func extractFencedCodeblock(data []byte, fence string) ([]byte, bool) {
	lines := strings.Split(string(data), "\n")

	var inBlock bool
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if !inBlock {
			if trimmed == fence {
				inBlock = true
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			return []byte(strings.Join(result, "\n")), true
		}
		result = append(result, trimmed)
	}
	return nil, false
}

// extractArrowReadme returns the markdown content surrounding the fenced
// ```arrow block (both before and after it), trimmed. ok is false when there
// is nothing to show: the input has no ```arrow fence at all, the fence is
// never closed, or the fence is the entire file with no prose around it.
func extractArrowReadme(data []byte) (string, bool) {
	return extractReadme(data, "```arrow")
}

func extractReadme(data []byte, fence string) (string, bool) {
	lines := strings.Split(string(data), "\n")

	var inBlock, closed bool
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		switch {
		case inBlock && strings.HasPrefix(trimmed, "```"):
			inBlock = false
			closed = true
		case inBlock:
			// Skip fenced content; it belongs to the manifest, not the readme.
		case trimmed == fence:
			inBlock = true
		default:
			result = append(result, trimmed)
		}
	}

	if !closed {
		return "", false
	}

	readme := strings.TrimSpace(strings.Join(result, "\n"))
	if readme == "" {
		return "", false
	}
	return readme, true
}
