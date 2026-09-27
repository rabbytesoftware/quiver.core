package picker

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPattern_Bounded_TokenBoundaries(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "whole", input: "debug", want: true},
		{name: "dash separated", input: "tool-debug-linux", want: true},
		{name: "underscore separated", input: "tool_debug", want: true},
		{name: "dot separated", input: "tool.debug.zip", want: true},
		{name: "slash start", input: "dir/debug", want: true},
		{name: "prefix of word", input: "debugger"},
		{name: "suffix of word", input: "nodebug"},
	}

	re := bounded("debug")
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, re.MatchString(tc.input))
		})
	}
}

func TestPattern_Suffixed_QuotesAndAnchors(t *testing.T) {
	re := suffixed(".exe")

	assert.True(t, re.MatchString("tool.exe"))
	assert.False(t, re.MatchString("tool.exe.zip"))
	assert.False(t, re.MatchString("toolxexe"))
}

func TestPattern_FirstMatch_Order(t *testing.T) {
	patterns := []pattern{
		{label: "a", re: regexp.MustCompile("x")},
		{label: "b", re: regexp.MustCompile("x|y")},
	}

	testCases := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "first wins", input: "x", want: "a", wantOK: true},
		{name: "second", input: "y", want: "b", wantOK: true},
		{name: "none", input: "z"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := firstMatch(patterns, tc.input)

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPattern_AllMatches_Collects(t *testing.T) {
	patterns := []pattern{
		{label: "a", re: regexp.MustCompile("x")},
		{label: "b", re: regexp.MustCompile("x|y")},
	}

	testCases := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "both", input: "x", want: []string{"a", "b"}},
		{name: "one", input: "y", want: []string{"b"}},
		{name: "none", input: "z"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, allMatches(patterns, tc.input))
		})
	}
}
