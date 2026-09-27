package readme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBadgeSrc_KnownHosts(
	t *testing.T,
) {
	testCases := []struct {
		name string
		src  string
		want bool
	}{
		{name: "shields.io", src: "https://img.shields.io/crates/v/ripgrep.svg", want: true},
		{name: "github actions badge.svg", src: "https://github.com/BurntSushi/ripgrep/workflows/ci/badge.svg", want: true},
		{name: "repology badge", src: "https://repology.org/badge/tiny-repos/ripgrep.svg", want: true},
		{name: "codecov", src: "https://codecov.io/gh/x/y/branch/main/graph/badge.svg", want: true},
		{name: "generic badge keyword", src: "https://example.com/some-badge-thing.png", want: true},
		{name: "unrelated screenshot host", src: "https://burntsushi.net/stuff/ripgrep1.png", want: false},
		{name: "unrelated imgur host", src: "https://imgur.com/rGsdnDe.png", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsBadgeSrc(tc.src))
		})
	}
}

func TestStripImageTokens_ClassifiesLine(
	t *testing.T,
) {
	testCases := []struct {
		name          string
		line          string
		wantRemainder string
		wantSawImage  bool
		wantBadgeHost bool
		wantNonBadge  bool
	}{
		{
			name:          "single linked badge image",
			line:          "[![Build](https://img.shields.io/x.svg)](https://x)",
			wantRemainder: "",
			wantSawImage:  true,
			wantBadgeHost: true,
			wantNonBadge:  false,
		},
		{
			name:          "bare non badge image",
			line:          "![Screenshot](https://example.net/shot.png)",
			wantRemainder: "",
			wantSawImage:  true,
			wantBadgeHost: false,
			wantNonBadge:  true,
		},
		{
			name:          "no image at all",
			line:          "just some prose",
			wantRemainder: "just some prose",
			wantSawImage:  false,
			wantBadgeHost: false,
			wantNonBadge:  false,
		},
		{
			name:          "image plus trailing prose",
			line:          "![Screenshot](https://example.net/shot.png) is neat",
			wantRemainder: " is neat",
			wantSawImage:  true,
			wantBadgeHost: false,
			wantNonBadge:  true,
		},
		{
			name:          "linked non badge image",
			line:          "[![Screenshot](https://example.net/x.png)](https://example.net/x.png)",
			wantRemainder: "",
			wantSawImage:  true,
			wantBadgeHost: false,
			wantNonBadge:  true,
		},
		{
			name:          "badge and non badge image mixed",
			line:          "[![Build](https://img.shields.io/x.svg)](https://x) ![Logo](logo.png)",
			wantRemainder: " ",
			wantSawImage:  true,
			wantBadgeHost: true,
			wantNonBadge:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			remainder, sawImage, sawBadgeHost, sawNonBadge := stripImageTokens(tc.line)
			assert.Equal(t, tc.wantRemainder, remainder)
			assert.Equal(t, tc.wantSawImage, sawImage)
			assert.Equal(t, tc.wantBadgeHost, sawBadgeHost)
			assert.Equal(t, tc.wantNonBadge, sawNonBadge)
		})
	}
}

func TestStripHTMLBadgeTokens_ClassifiesLine(
	t *testing.T,
) {
	testCases := []struct {
		name          string
		line          string
		wantRemainder string
		wantSawImage  bool
		wantBadgeHost bool
		wantNonBadge  bool
	}{
		{
			name:          "bare badge img",
			line:          `<img src="https://img.shields.io/crates/l/bat.svg" alt="license">`,
			wantRemainder: "",
			wantSawImage:  true,
			wantBadgeHost: true,
			wantNonBadge:  false,
		},
		{
			name:          "anchor wrapped badge img",
			line:          `<a href="https://x"><img src="https://github.com/x/y/workflows/CICD/badge.svg" alt="Build Status"></a>`,
			wantRemainder: "",
			wantSawImage:  true,
			wantBadgeHost: true,
			wantNonBadge:  false,
		},
		{
			name:          "badge img with trailing br",
			line:          `<img src="https://img.shields.io/x.svg"><br>`,
			wantRemainder: "",
			wantSawImage:  true,
			wantBadgeHost: true,
			wantNonBadge:  false,
		},
		{
			name:          "non badge img",
			line:          `<img src="doc/logo-header.svg" alt="logo">`,
			wantRemainder: "",
			wantSawImage:  true,
			wantBadgeHost: false,
			wantNonBadge:  true,
		},
		{
			name:          "badge and non badge img mixed",
			line:          `<img src="doc/logo-header.svg"><img src="https://img.shields.io/x.svg">`,
			wantRemainder: "",
			wantSawImage:  true,
			wantBadgeHost: true,
			wantNonBadge:  true,
		},
		{
			name:          "no image",
			line:          "A cat clone with syntax highlighting.",
			wantRemainder: "A cat clone with syntax highlighting.",
			wantSawImage:  false,
			wantBadgeHost: false,
			wantNonBadge:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			remainder, sawImage, sawBadgeHost, sawNonBadge := stripHTMLBadgeTokens(tc.line)
			assert.Equal(t, tc.wantRemainder, remainder)
			assert.Equal(t, tc.wantSawImage, sawImage)
			assert.Equal(t, tc.wantBadgeHost, sawBadgeHost)
			assert.Equal(t, tc.wantNonBadge, sawNonBadge)
		})
	}
}

func TestIsBadgeOnlyLine_RequiresBadgeHost(
	t *testing.T,
) {
	testCases := []struct {
		name string
		line string
		want bool
	}{
		{name: "badge only", line: "[![Build](https://img.shields.io/x.svg)](https://x)", want: true},
		{name: "screenshot only, not a badge host", line: "![Screenshot](https://example.net/shot.png)", want: false},
		{name: "badge with trailing text", line: "[![Build](https://img.shields.io/x.svg)](https://x) done", want: false},
		{name: "no image", line: "plain text", want: false},
		{
			name: "html badge only",
			line: `<img src="https://img.shields.io/crates/l/bat.svg" alt="license">`,
			want: true,
		},
		{
			name: "html badge wrapped in anchor and br",
			line: `<a href="https://x"><img src="https://github.com/x/y/workflows/CICD/badge.svg"></a><br>`,
			want: true,
		},
		{
			name: "html badge mixed with logo image stays",
			line: `<img src="https://img.shields.io/x.svg"><img src="doc/logo-header.svg">`,
			want: false,
		},
		{
			name: "html non badge image only",
			line: `<img src="doc/logo-header.svg" alt="logo">`,
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isBadgeOnlyLine(tc.line))
		})
	}
}

func TestStripBadgeLines_RemovesOnlyBadgeOnlyLines(
	t *testing.T,
) {
	lines := []string{
		"prose before",
		"[![Build](https://img.shields.io/x.svg)](https://x)",
		"![Screenshot](https://example.net/shot.png)",
		`<img src="https://img.shields.io/crates/l/bat.svg" alt="license">`,
		"prose after",
	}

	out := stripBadgeLines(lines)

	assert.Equal(t, []string{
		"prose before",
		"![Screenshot](https://example.net/shot.png)",
		"prose after",
	}, out)
}

func TestStripBadgeLines_IgnoresLinesInsideFence(
	t *testing.T,
) {
	lines := []string{
		"```",
		"[![Build](https://img.shields.io/x.svg)](https://x)",
		"```",
	}

	out := stripBadgeLines(lines)

	assert.Equal(t, lines, out)
}

func TestHtmlBlockTag_DetectsOpeningPOrDiv(
	t *testing.T,
) {
	testCases := []struct {
		name    string
		line    string
		wantTag string
		wantOK  bool
	}{
		{name: "p with align", line: `<p align="center">`, wantTag: "p", wantOK: true},
		{name: "div plain", line: "<div>", wantTag: "div", wantOK: true},
		{name: "uppercase P", line: `<P ALIGN="center">`, wantTag: "p", wantOK: true},
		{name: "not a block tag", line: "<span>", wantOK: false},
		{name: "self closing img is not a block", line: `<img src="x.png">`, wantOK: false},
		{name: "prose", line: "hello", wantOK: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tag, ok := htmlBlockTag(tc.line)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantTag, tag)
			}
		})
	}
}

func TestStripHTMLBadgeBlocks_RemovesAllBadgeBlockEntirely(
	t *testing.T,
) {
	lines := []string{
		"prose before",
		`<p align="center">`,
		`  <a href="https://x"><img src="https://img.shields.io/a.svg"></a>`,
		`  <a href="https://y"><img src="https://img.shields.io/b.svg"></a>`,
		`  <a href="https://z"><img src="https://img.shields.io/c.svg"></a>`,
		"</p>",
		"prose after",
	}

	out := stripHTMLBadgeBlocks(lines)

	assert.Equal(t, []string{"prose before", "prose after"}, out)
}

func TestStripHTMLBadgeBlocks_ToleratesBlankAndBareBrFillerLines(
	t *testing.T,
) {
	lines := []string{
		"prose before",
		`<p align="center">`,
		`  <img src="https://img.shields.io/a.svg">`,
		"",
		"  <br>",
		`  <img src="https://img.shields.io/b.svg">`,
		"</p>",
		"prose after",
	}

	out := stripHTMLBadgeBlocks(lines)

	assert.Equal(t, []string{"prose before", "prose after"}, out)
}

func TestStripHTMLBadgeBlocks_MixedBlockStaysUntouched(
	t *testing.T,
) {
	lines := []string{
		`<p align="center">`,
		`  <img src="doc/logo-header.svg" alt="logo"><br>`,
		`  <img src="https://img.shields.io/x.svg" alt="license">`,
		`  A <i>cat(1)</i> clone with syntax highlighting.`,
		"</p>",
	}

	out := stripHTMLBadgeBlocks(lines)

	assert.Equal(t, lines, out)
}

func TestStripHTMLBadgeBlocks_UnterminatedBlockLeftUntouched(
	t *testing.T,
) {
	lines := []string{
		`<p align="center">`,
		`  <img src="https://img.shields.io/a.svg">`,
	}

	out := stripHTMLBadgeBlocks(lines)

	assert.Equal(t, lines, out)
}

func TestStripHTMLBadgeBlocks_IgnoresBlockOpenerInsideFence(
	t *testing.T,
) {
	lines := []string{
		"```",
		`<p align="center">`,
		`<img src="https://img.shields.io/a.svg">`,
		"</p>",
		"```",
	}

	out := stripHTMLBadgeBlocks(lines)

	assert.Equal(t, lines, out)
}

func TestStripHTMLBadgeBlocks_DivBlockAlsoRemoved(
	t *testing.T,
) {
	lines := []string{
		"<div>",
		`<img src="https://img.shields.io/a.svg">`,
		"</div>",
	}

	out := stripHTMLBadgeBlocks(lines)

	assert.Empty(t, out)
}
