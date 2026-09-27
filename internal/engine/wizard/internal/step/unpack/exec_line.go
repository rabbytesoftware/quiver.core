package unpack

import (
	"strings"
)

const fieldCodes = "fFuUdDnNickvm"

func parseExec(
	line string,
) (string, []string) {
	tokens := splitExec(line)
	if len(tokens) == 0 {
		return "", nil
	}

	args := make([]string, 0, len(tokens)-1)
	for _, token := range tokens[1:] {
		if isFieldCode(token) {
			continue
		}
		args = append(args, strings.ReplaceAll(token, "%%", "%"))
	}

	return tokens[0], args
}

func splitExec(
	line string,
) []string {
	var tokens []string
	var current strings.Builder
	started := false
	quoted := false

	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quoted && r == '\\' && i+1 < len(runes) && isQuotedEscape(runes[i+1]):
			i++
			current.WriteRune(runes[i])
		case r == '"':
			quoted = !quoted
			started = true
		case !quoted && (r == ' ' || r == '\t'):
			if started {
				tokens = append(tokens, current.String())
			}
			current.Reset()
			started = false
		default:
			current.WriteRune(r)
			started = true
		}
	}

	if started {
		tokens = append(tokens, current.String())
	}

	return tokens
}

func isQuotedEscape(
	r rune,
) bool {
	return r == '"' || r == '`' || r == '$' || r == '\\'
}

func isFieldCode(
	token string,
) bool {
	return len(token) == 2 && token[0] == '%' && strings.IndexByte(fieldCodes, token[1]) >= 0
}
