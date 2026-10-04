package command_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/console/command"
)

func TestTokenize_Splits(t *testing.T) {
	cases := map[string][]string{
		`install github.com/a/b`:         {"install", "github.com/a/b"},
		"  install   x\tdetach  ":        {"install", "x", "detach"},
		`run x --data "k=a b"`:           {"run", "x", "--data", "k=a b"},
		`run x --data 'k=a "b"'`:         {"run", "x", "--data", `k=a "b"`},
		`say "a \"quoted\" word"`:        {"say", `a "quoted" word`},
		`say "back\\slash"`:              {"say", `back\slash`},
		`say "keep \n literal"`:          {"say", `keep \n literal`},
		`say a\ b`:                       {"say", "a b"},
		`say a"b c"d`:                    {"say", "ab cd"},
		`say ""`:                         {"say", ""},
		`say '' x`:                       {"say", "", "x"},
		`say $HOME ` + "`id`" + ` $(id)`: {"say", "$HOME", "`id`", "$(id)"},
		`say a;b|c&d>e<f*g?h~i`:          {"say", "a;b|c&d>e<f*g?h~i"},
		`say 'single \ stays'`:           {"say", `single \ stays`},
		`say é "ü ñ"`:                    {"say", "é", "ü ñ"},
		`x ` + strings.Repeat("a", 500):  {"x", strings.Repeat("a", 500)},
	}
	for line, want := range cases {
		got, err := command.Tokenize(line)
		require.NoError(t, err, line)
		assert.Equal(t, want, got, line)
	}
}

func TestTokenize_Rejects(t *testing.T) {
	cases := map[string]string{
		"":                        "empty",
		"   \t ":                  "empty",
		"install x\n":             "control",
		"install x\r\nrun y":      "control",
		"install\x00x":            "control",
		"install\x1bx":            "control",
		"install\x7fx":            "control",
		`install "unterminated`:   "unterminated",
		`install 'unterminated`:   "unterminated",
		`install trailing\`:       "trailing",
		`install "trailing\`:      "unterminated",
		"install \xff\xfe":        "UTF-8",
		strings.Repeat("a", 1025): "too long",
		strings.Repeat("a ", 65):  "too many",
	}
	for line, reason := range cases {
		_, err := command.Tokenize(line)
		var lineErr *command.LineError
		require.True(t, errors.As(err, &lineErr), "%q must be a LineError, got %v", line, err)
		assert.Contains(t, err.Error(), reason, line)
	}
}

func TestTokenize_LimitsAreInclusive(t *testing.T) {
	_, err := command.Tokenize(strings.Repeat("a", command.MaxLineBytes))
	require.NoError(t, err)

	tokens, err := command.Tokenize(strings.TrimSpace(strings.Repeat("a ", command.MaxTokens)))
	require.NoError(t, err)
	assert.Len(t, tokens, command.MaxTokens)
}
