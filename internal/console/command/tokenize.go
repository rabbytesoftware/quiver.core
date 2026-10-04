package command

import "unicode/utf8"

// Tokenize splits a console line into arguments.
//
// Arguments are separated by spaces or tabs. Single quotes keep everything
// literal. Inside double quotes a backslash escapes only a double quote or a
// backslash. Outside quotes a backslash escapes the next character. Nothing
// else is special: $, backticks, ;, |, &, <, >, *, ? and ~ are ordinary
// characters, because the result is never handed to a shell.
//
// It returns a *LineError for an empty line, a line over MaxLineBytes, more
// than MaxTokens arguments, control characters (including newlines and NUL),
// invalid UTF-8, an unterminated quote and a trailing backslash.
func Tokenize(
	line string,
) ([]string, error) {
	if len(line) > MaxLineBytes {
		return nil, &LineError{Reason: "command line is too long"}
	}
	if !utf8.ValidString(line) {
		return nil, &LineError{Reason: "command line is not valid UTF-8"}
	}

	lex := &lexer{}
	for _, r := range line {
		lex.feed(r)
	}

	tokens, err := lex.finish()
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, &LineError{Reason: "empty command line"}
	}
	return tokens, nil
}
