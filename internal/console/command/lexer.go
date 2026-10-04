package command

import "strings"

type escapeMode int

const (
	escapeNone escapeMode = iota
	escapeBare
	escapeDouble
)

type lexer struct {
	err     error
	tokens  []string
	current strings.Builder
	started bool
	quote   rune
	escape  escapeMode
}

func (l *lexer) feed(
	r rune,
) {
	if l.err != nil {
		return
	}
	if isControl(r) {
		l.err = &LineError{Reason: "control characters are not allowed"}
		return
	}
	if l.escape != escapeNone {
		l.applyEscape(r)
		return
	}

	switch l.quote {
	case '\'':
		l.single(r)
	case '"':
		l.double(r)
	default:
		l.bare(r)
	}
}

func (l *lexer) applyEscape(
	r rune,
) {
	mode := l.escape
	l.escape = escapeNone
	if mode == escapeDouble && r != '"' && r != '\\' {
		l.current.WriteRune('\\')
	}
	l.current.WriteRune(r)
}

func (l *lexer) single(
	r rune,
) {
	if r == '\'' {
		l.quote = 0
		return
	}
	l.current.WriteRune(r)
}

func (l *lexer) double(
	r rune,
) {
	switch r {
	case '"':
		l.quote = 0
	case '\\':
		l.escape = escapeDouble
	default:
		l.current.WriteRune(r)
	}
}

func (l *lexer) bare(
	r rune,
) {
	switch r {
	case ' ', '\t':
		l.push()
	case '\'', '"':
		l.quote = r
		l.started = true
	case '\\':
		l.escape = escapeBare
		l.started = true
	default:
		l.current.WriteRune(r)
		l.started = true
	}
}

func (l *lexer) push() {
	if !l.started {
		return
	}
	if len(l.tokens) >= MaxTokens {
		l.err = &LineError{Reason: "too many arguments"}
		return
	}

	l.tokens = append(l.tokens, l.current.String())
	l.current.Reset()
	l.started = false
}

func (l *lexer) finish() ([]string, error) {
	if l.err != nil {
		return nil, l.err
	}
	if l.quote != 0 {
		return nil, &LineError{Reason: "unterminated quote"}
	}
	if l.escape != escapeNone {
		return nil, &LineError{Reason: "trailing backslash"}
	}

	l.push()
	if l.err != nil {
		return nil, l.err
	}
	return l.tokens, nil
}

func isControl(
	r rune,
) bool {
	return r == 0 || r == 0x7f || (r < 0x20 && r != '\t')
}
