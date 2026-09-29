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

func TestTruncate_LeavesShortTextUntouched(
	t *testing.T,
) {
	assert.Equal(t, "short", truncate("short", 100))
}

func TestTruncate_CutsAtRuneLimit(
	t *testing.T,
) {
	assert.Equal(t, "ab", truncate("abcdef", 2))
}

func TestTruncate_IsRuneSafeOnMultiByteText(
	t *testing.T,
) {
	got := truncate("日本語のテキスト", 3)

	assert.Equal(t, "日本語", got)
}

func TestTruncate_ByteLengthOverLimitButRuneCountUnder(
	t *testing.T,
) {
	got := truncate("日本語", 5)

	assert.Equal(t, "日本語", got)
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
