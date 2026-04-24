package translator

import "strings"

// extractArrowCodeblock extracts the content of the first ```arrow fenced block
// from markdown input. Returns the YAML bytes and true if found, nil and false otherwise.
func extractArrowCodeblock(data []byte) ([]byte, bool) {
	const fence = "```arrow"
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
