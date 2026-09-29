package readme

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsFenceDelim_VariousLines(
	t *testing.T,
) {
	testCases := []struct {
		name string
		line string
		want bool
	}{
		{name: "backtick fence", line: "```", want: true},
		{name: "backtick fence with info", line: "```go", want: true},
		{name: "tilde fence", line: "~~~", want: true},
		{name: "indented fence", line: "  ```bash", want: true},
		{name: "prose line", line: "hello world", want: false},
		{name: "empty line", line: "", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isFenceDelim(tc.line))
		})
	}
}

func TestFenceStates_TracksToggle(
	t *testing.T,
) {
	lines := []string{
		"prose",
		"```",
		"# not a heading",
		"```",
		"prose again",
	}

	states := fenceStates(lines)

	require.Len(t, states, 5)
	assert.False(t, states[0])
	assert.True(t, states[1])
	assert.True(t, states[2])
	assert.True(t, states[3])
	assert.False(t, states[4])
}

func TestNeutralizeFences_ConvertsBacktickLeadFences(
	t *testing.T,
) {
	testCases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "plain fence pair",
			in:   []string{"```", "code", "```"},
			want: []string{"~~~", "code", "~~~"},
		},
		{
			name: "fence with info string",
			in:   []string{"```arrow", "schema: x", "```"},
			want: []string{"~~~arrow", "schema: x", "~~~"},
		},
		{
			name: "indented fence",
			in:   []string{"  ```bash", "cmd"},
			want: []string{"  ~~~bash", "cmd"},
		},
		{
			name: "already tilde fence untouched",
			in:   []string{"~~~", "code", "~~~"},
			want: []string{"~~~", "code", "~~~"},
		},
		{
			name: "non fence untouched",
			in:   []string{"prose here"},
			want: []string{"prose here"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, neutralizeFences(tc.in))
		})
	}
}

func TestNeutralizeFences_NoLiteralArrowFenceSurvives(
	t *testing.T,
) {
	lines := []string{"prose", "```arrow", "schema: x", "```", "more prose"}

	out := neutralizeFences(lines)

	for _, line := range out {
		assert.NotEqual(t, "```arrow", line)
		assert.False(t, strings.HasPrefix(strings.TrimSpace(line), "```"))
	}
}

func TestIsLeadingVisualLine_Classification(
	t *testing.T,
) {
	testCases := []struct {
		name string
		line string
		want bool
	}{
		{name: "blank", line: "   ", want: true},
		{name: "hr dashes", line: "---", want: true},
		{name: "hr stars", line: "***", want: true},
		{name: "badge line", line: "[![Build](https://img.shields.io/x.svg)](https://x)", want: true},
		{name: "prose", line: "this is real content", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isLeadingVisualLine(tc.line))
		})
	}
}

func TestStripLeadingBlock_RemovesSetextH1AndBadges(
	t *testing.T,
) {
	lines := []string{
		"Title",
		"-----",
		"[![Build](https://img.shields.io/x.svg)](https://x)",
		"",
		"Real prose starts here.",
	}

	out := stripLeadingBlock(lines)

	assert.Equal(t, []string{"Real prose starts here."}, out)
}

func TestStripLeadingBlock_RemovesAtxH1(
	t *testing.T,
) {
	lines := []string{
		"# Title",
		"",
		"Real prose starts here.",
	}

	out := stripLeadingBlock(lines)

	assert.Equal(t, []string{"Real prose starts here."}, out)
}

func TestStripLeadingBlock_NoHeadingLeavesLinesUntouched(
	t *testing.T,
) {
	lines := []string{
		"<p align=\"center\">",
		"real content",
		"</p>",
	}

	out := stripLeadingBlock(lines)

	assert.Equal(t, lines, out)
}

func TestStripLeadingBlock_AllBlankLinesLeavesLinesUntouched(
	t *testing.T,
) {
	lines := []string{"", "  ", ""}

	out := stripLeadingBlock(lines)

	assert.Equal(t, lines, out)
}

func TestStripLeadingBlock_EqualsUnderlineIsNotTreatedAsLeadingTitle(
	t *testing.T,
) {
	lines := []string{
		"Title",
		"=====",
		"prose",
	}

	out := stripLeadingBlock(lines)

	assert.Equal(t, lines, out)
}

func TestStripLeadingBlock_NonH1AtxHeadingLeavesUntouched(
	t *testing.T,
) {
	lines := []string{
		"## Not an H1",
		"prose",
	}

	out := stripLeadingBlock(lines)

	assert.Equal(t, lines, out)
}

func TestStripInstallSections_RemovesUntilSameLevelHeading(
	t *testing.T,
) {
	lines := []string{
		"### Intro",
		"keep me",
		"### Installation",
		"drop me",
		"drop me too",
		"### Building",
		"keep me too",
	}

	out := stripInstallSections(lines)

	assert.Equal(t, []string{
		"### Intro",
		"keep me",
		"### Building",
		"keep me too",
	}, out)
}

func TestStripInstallSections_HigherLevelHeadingAlsoEndsSection(
	t *testing.T,
) {
	lines := []string{
		"## Installation",
		"drop me",
		"# Top Level",
		"keep me",
	}

	out := stripInstallSections(lines)

	assert.Equal(t, []string{
		"# Top Level",
		"keep me",
	}, out)
}

func TestStripInstallSections_LowerLevelHeadingStaysDropped(
	t *testing.T,
) {
	lines := []string{
		"## Installation",
		"### Windows",
		"drop me",
		"## Next Section",
		"keep me",
	}

	out := stripInstallSections(lines)

	assert.Equal(t, []string{
		"## Next Section",
		"keep me",
	}, out)
}

func TestStripInstallSections_RunsToEndOfDocument(
	t *testing.T,
) {
	lines := []string{
		"## Installation",
		"drop me",
	}

	out := stripInstallSections(lines)

	assert.Empty(t, out)
}

func TestStripInstallSections_IgnoresHeadingsInsideFence(
	t *testing.T,
) {
	lines := []string{
		"prose",
		"```",
		"## Installation",
		"```",
		"prose again",
	}

	out := stripInstallSections(lines)

	assert.Equal(t, lines, out)
}

func TestStripInstallSections_MatchesDownloadHeading(
	t *testing.T,
) {
	lines := []string{
		"## Downloads",
		"drop me",
		"## Next",
		"keep me",
	}

	out := stripInstallSections(lines)

	assert.Equal(t, []string{"## Next", "keep me"}, out)
}

func TestNeutralizeFences_Text(
	t *testing.T,
) {
	testCases := []struct {
		name string
		text string
		want string
	}{
		{name: "plain text untouched", text: "hello\nworld", want: "hello\nworld"},
		{name: "arrow fence neutralised", text: "```arrow", want: "~~~arrow"},
		{name: "indented fence keeps indent", text: "a\n  ```\nb", want: "a\n  ~~~\nb"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NeutralizeFences(tc.text))
		})
	}
}
