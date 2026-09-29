package readme

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestSplitLines_SplitsOnNewline(
	t *testing.T,
) {
	assert.Equal(t, []string{"a", "b", "c"}, splitLines([]byte("a\nb\nc")))
}

func TestCollapseBlankLines_KeepsAtMostOneBlankInARow(
	t *testing.T,
) {
	in := []string{"a", "", "", "", "b", "c", "", "d"}

	out := collapseBlankLines(in)

	assert.Equal(t, []string{"a", "", "b", "c", "", "d"}, out)
}

func TestTruncate_Cases(
	t *testing.T,
) {
	testCases := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{name: "short text untouched", text: "short", limit: 100, want: "short"},
		{name: "cuts at the rune limit", text: "abcdef", limit: 2, want: "ab"},
		{name: "rune safe on multi byte text", text: "日本語のテキスト", limit: 3, want: "日本語"},
		{name: "byte length over the limit but rune count under", text: "日本語", limit: 5, want: "日本語"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, truncate(tc.text, tc.limit))
		})
	}
}

func TestTransform_RipgrepReadmeFixture(
	t *testing.T,
) {
	raw, err := os.ReadFile("testdata/ripgrep_readme.md")
	require.NoError(t, err)
	base := githubBase("BurntSushi", "ripgrep", "14.1.0")

	out := Transform(raw, base)

	assert.NotContains(t, out, "ripgrep (rg)\n------------")
	assert.NotContains(t, out, "img.shields.io")
	assert.NotContains(t, out, "badge.svg")
	assert.NotContains(t, out, "repology.org/badge")
	assert.NotContains(t, out, "### Installation")
	assert.NotContains(t, out, "brew install ripgrep")
	assert.Contains(t, out, "### Building")
	assert.Contains(t, out, "https://github.com/BurntSushi/ripgrep/blob/14.1.0/CHANGELOG.md")
	assert.NotContains(t, out, "```")
	assert.Contains(t, out, "~~~")
}

func TestTransform_BatReadmeFixture(
	t *testing.T,
) {
	raw, err := os.ReadFile("testdata/bat_readme.md")
	require.NoError(t, err)
	base := githubBase("sharkdp", "bat", "v0.25.0")

	out := Transform(raw, base)

	assert.Contains(t, out, `<img src="https://raw.githubusercontent.com/sharkdp/bat/v0.25.0/doc/logo-header.svg"`)
	assert.Contains(t, out, "A <i>cat(1)</i> clone with syntax highlighting")
	assert.NotContains(t, out, "img.shields.io/crates/l/bat.svg")
	assert.NotContains(t, out, "workflows/CICD/badge.svg")
	assert.NotContains(t, out, "```arrow")
	assert.Contains(t, out, "~~~arrow")
	assert.NotContains(t, out, "## Installation")
	assert.NotContains(t, out, "apt install bat")
	assert.Contains(t, out, "## Customization")
}

func TestTransform_TruncatesToMaxReadmeLength(
	t *testing.T,
) {
	huge := strings.Repeat("a", domain.MaxReadmeLength+500)

	out := Transform([]byte(huge), githubBase("o", "r", "v1"))

	assert.LessOrEqual(t, len([]rune(out)), domain.MaxReadmeLength)
}

func TestTransform_EmptyInputReturnsEmpty(
	t *testing.T,
) {
	out := Transform([]byte(""), githubBase("o", "r", "v1"))

	assert.Empty(t, out)
}

func TestTransform_Cases(
	t *testing.T,
) {
	base := githubBase("o", "r", "v1")
	testCases := []struct {
		name string
		in   string
		want string
	}{
		{name: "setext title and badges", in: "Tool\n----\n[![ci](https://img.shields.io/x.svg)](https://ci)\n***\nBody", want: "Body"},
		{name: "equals underline kept", in: "Tool\n====\nBody", want: "Tool\n====\nBody"},
		{name: "h2 kept", in: "## Tool\nBody", want: "## Tool\nBody"},
		{name: "html badge block", in: "<p align=\"center\">\n<a href=\"x\"><img src=\"https://badgen.net/x\"></a><br>\n\n</p>\nBody", want: "Body"},
		{name: "div badge block", in: "<div>\n<img src=\"https://img.shields.io/a\">\n</div>\nBody", want: "Body"},
		{name: "mixed html block kept", in: "<p>\n<img src=\"logo.png\">\n</p>", want: "<p>\n<img src=\"https://raw.githubusercontent.com/o/r/v1/logo.png\">\n</p>"},
		{name: "unterminated block kept", in: "<p>\n<img src=\"https://img.shields.io/a\">", want: "<p>"},
		{name: "block opener in fence kept", in: "~~~\n<p>\n</p>\n~~~", want: "~~~\n<p>\n</p>\n~~~"},
		{name: "links", in: "[a](docs/a.md) [b](#x) [c](mailto:x) [d](//h/x) [e](https://h/x) ![i](./i.png)", want: "[a](https://github.com/o/r/blob/v1/docs/a.md) [b](#x) [c](mailto:x) [d](//h/x) [e](https://h/x) ![i](https://raw.githubusercontent.com/o/r/v1/i.png)"},
		{name: "install section dropped", in: "Intro\n## Install\nx\n### sub\ny\n## Usage\nz", want: "Intro\n## Usage\nz"},
		{name: "fenced heading kept", in: "```\n# Install\n```", want: "~~~\n# Install\n~~~"},
		{name: "all blank", in: "\n\n", want: ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Transform([]byte(tc.in), base))
		})
	}
	assert.Equal(t, "x", Transform([]byte("x"), RawBase{}))
	assert.Equal(t, "[a](a.md)", Transform([]byte("[a](a.md)"), RawBase{}))
	assert.Equal(t, "~~~arrow", NeutralizeFences("```arrow"))
}
