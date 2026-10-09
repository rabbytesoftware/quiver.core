package domain

import "strings"

// SearchTerms splits a free-text query into the terms a search must all find.
// Whitespace, '.', '-' and '_' all separate terms, so quiver-chat, quiver.chat
// and quiver_chat ask the same question and each finds the other's spelling.
func SearchTerms(
	text string,
) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '.', '-', '_':
			return true
		}
		return r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}
