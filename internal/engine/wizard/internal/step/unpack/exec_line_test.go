package unpack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseExec_SplitsProgramAndArgs(t *testing.T) {
	testCases := []struct {
		name        string
		line        string
		wantProgram string
		wantArgs    []string
	}{
		{name: "apprun with field code", line: "AppRun --no-sandbox %U", wantProgram: "AppRun", wantArgs: []string{"--no-sandbox"}},
		{name: "quoted program and argument", line: `"bin/app" --flag="a b" %f`, wantProgram: "bin/app", wantArgs: []string{"--flag=a b"}},
		{name: "literal percent", line: "app %% %u", wantProgram: "app", wantArgs: []string{"%"}},
		{name: "escaped percent inside a token", line: "app --fmt=%%s", wantProgram: "app", wantArgs: []string{"--fmt=%s"}},
		{name: "absolute program keeps its args for the caller to drop", line: "/usr/bin/app x", wantProgram: "/usr/bin/app", wantArgs: []string{"x"}},
		{name: "escapes inside quotes", line: `app "say \"hi\"" "c:\\dir" "\$HOME" "\` + "`" + `cmd\` + "`" + `"`, wantProgram: "app", wantArgs: []string{`say "hi"`, `c:\dir`, "$HOME", "`cmd`"}},
		{name: "unknown escape inside quotes is literal", line: `app "a\nb"`, wantProgram: "app", wantArgs: []string{`a\nb`}},
		{name: "backslash outside quotes is literal", line: `app a\"b`, wantProgram: "app", wantArgs: []string{`a\b`}},
		{name: "empty quoted argument", line: `app ""`, wantProgram: "app", wantArgs: []string{""}},
		{name: "every field code is dropped", line: "app %f %F %u %U %d %D %n %N %i %c %k %v %m", wantProgram: "app", wantArgs: []string{}},
		{name: "repeated whitespace and tabs", line: "  app \t one   two  ", wantProgram: "app", wantArgs: []string{"one", "two"}},
		{name: "unterminated quote runs to the end", line: `app "open ended`, wantProgram: "app", wantArgs: []string{"open ended"}},
		{name: "empty line", line: "", wantProgram: "", wantArgs: nil},
		{name: "only whitespace", line: "   ", wantProgram: "", wantArgs: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			program, args := parseExec(tc.line)

			assert.Equal(t, tc.wantProgram, program)
			assert.Equal(t, tc.wantArgs, args)
		})
	}
}
