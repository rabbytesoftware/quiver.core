package readme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRelativePath_Classification(
	t *testing.T,
) {
	testCases := []struct {
		name string
		url  string
		want bool
	}{
		{name: "https absolute", url: "https://example.com/x.png", want: false},
		{name: "http absolute", url: "http://example.com/x.png", want: false},
		{name: "anchor", url: "#installation", want: false},
		{name: "mailto", url: "mailto:foo@example.com", want: false},
		{name: "tel", url: "tel:+123456", want: false},
		{name: "protocol relative", url: "//cdn.example.com/img.png", want: false},
		{name: "protocol relative single leading slash is still relative", url: "/cdn.example.com/img.png", want: true},
		{name: "relative path", url: "doc/logo.svg", want: true},
		{name: "relative dot path", url: "./doc/logo.svg", want: true},
		{name: "relative rooted path", url: "/doc/logo.svg", want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRelativePath(tc.url))
		})
	}
}

func TestCleanRelativePath_StripsDotAndSlashPrefix(
	t *testing.T,
) {
	testCases := []struct {
		name string
		path string
		want string
	}{
		{name: "dot slash prefix", path: "./doc/logo.svg", want: "doc/logo.svg"},
		{name: "rooted prefix", path: "/doc/logo.svg", want: "doc/logo.svg"},
		{name: "already clean", path: "doc/logo.svg", want: "doc/logo.svg"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, cleanRelativePath(tc.path))
		})
	}
}

func TestRawImageURL_PinsAtRef(
	t *testing.T,
) {
	base := RawBase{Owner: "sharkdp", Repo: "bat", Ref: "v0.25.0"}

	got := RawImageURL(base, "./doc/logo.svg")

	assert.Equal(t, "https://raw.githubusercontent.com/sharkdp/bat/v0.25.0/doc/logo.svg", got)
}

func TestBlobLinkURL_PinsAtRef(
	t *testing.T,
) {
	base := RawBase{Owner: "BurntSushi", Repo: "ripgrep", Ref: "14.1.0"}

	got := BlobLinkURL(base, "CHANGELOG.md")

	assert.Equal(t, "https://github.com/BurntSushi/ripgrep/blob/14.1.0/CHANGELOG.md", got)
}

func TestRewriteLine_RewritesRelativeImageAndLink(
	t *testing.T,
) {
	base := RawBase{Owner: "o", Repo: "r", Ref: "v1"}

	testCases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "markdown image relative",
			line: "![alt](doc/logo.svg)",
			want: "![alt](https://raw.githubusercontent.com/o/r/v1/doc/logo.svg)",
		},
		{
			name: "markdown link relative",
			line: "[CHANGELOG](CHANGELOG.md)",
			want: "[CHANGELOG](https://github.com/o/r/blob/v1/CHANGELOG.md)",
		},
		{
			name: "markdown image absolute untouched",
			line: "![alt](https://example.com/x.png)",
			want: "![alt](https://example.com/x.png)",
		},
		{
			name: "markdown link anchor untouched",
			line: "[Installation](#installation)",
			want: "[Installation](#installation)",
		},
		{
			name: "html img src relative",
			line: `<img src="doc/logo.svg" alt="logo">`,
			want: `<img src="https://raw.githubusercontent.com/o/r/v1/doc/logo.svg" alt="logo">`,
		},
		{
			name: "html img src absolute untouched",
			line: `<img src="https://example.com/x.png" alt="logo">`,
			want: `<img src="https://example.com/x.png" alt="logo">`,
		},
		{
			name: "markdown image protocol relative untouched",
			line: "![alt](//cdn.example.com/img.png)",
			want: "![alt](//cdn.example.com/img.png)",
		},
		{
			name: "html img src protocol relative untouched",
			line: `<img src="//cdn.example.com/img.png" alt="logo">`,
			want: `<img src="//cdn.example.com/img.png" alt="logo">`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, rewriteLine(tc.line, base))
		})
	}
}

func TestRewriteLinks_SkipsFencedLines(
	t *testing.T,
) {
	base := RawBase{Owner: "o", Repo: "r", Ref: "v1"}
	lines := []string{
		"```",
		"![alt](doc/logo.svg)",
		"```",
	}

	out := rewriteLinks(lines, base)

	assert.Equal(t, lines, out)
}

func TestImages_ExtractsMarkdownAndHTMLSources(
	t *testing.T,
) {
	raw := []byte("![a](one.png)\n<img src=\"two.png\">\n```\n![c](inside-fence.png)\n```\n")

	images := Images(raw)

	a := assert.New(t)
	a.Len(images, 2)
	a.Equal("one.png", images[0].Src)
	a.Equal(0, images[0].Line)
	a.Equal("two.png", images[1].Src)
	a.Equal(1, images[1].Line)
}

func TestImages_EmptyForNoImages(
	t *testing.T,
) {
	images := Images([]byte("just prose, no pictures here"))

	assert.Empty(t, images)
}
