package providers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const sampleHex = "4d83375d202ffa634eaf627fd9272b610fa599f20f5b711f55d770583eca2b84"

func TestSHA256Digest(t *testing.T) {
	testCases := []struct {
		name  string
		value string
		want  string
	}{
		{name: "normalised", value: " " + strings.ToUpper(sampleHex) + "\t", want: "sha256:" + sampleHex},
		{name: "not hex", value: strings.Repeat("z", 64)},
		{name: "sha1 length", value: sampleHex[:40]},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sha256Digest(tc.value))
		})
	}
}

func TestChecksumTarget(t *testing.T) {
	testCases := []struct {
		name       string
		asset      string
		wantTarget string
		wantOK     bool
	}{
		{name: "goreleaser list", asset: "checksums.txt", wantOK: true},
		{name: "upper case list", asset: "SHA256SUMS", wantOK: true},
		{name: "single asset sum", asset: "Tool.AppImage.sha256", wantTarget: "tool.appimage", wantOK: true},
		{name: "bare sha256 suffix", asset: ".sha256"},
		{name: "checksums without txt", asset: "checksums.json"},
		{name: "other algorithm list", asset: "sha512-checksums.txt"},
		{name: "regular asset", asset: "tool_linux_amd64.tar.gz"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target, ok := checksumTarget(tc.asset)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantTarget, target)
		})
	}
}

func TestParseChecksums(t *testing.T) {
	other := strings.Repeat("a", 64)
	testCases := []struct {
		name   string
		body   string
		target string
		want   map[string]string
	}{
		{
			name: "sha256sum text and binary markers",
			body: sampleHex + "  Tool_Linux.tar.gz\n" + other + " *tool.exe\n",
			want: map[string]string{
				"tool_linux.tar.gz": "sha256:" + sampleHex,
				"tool.exe":          "sha256:" + other,
			},
		},
		{
			name: "path prefixes collapse to the file name",
			body: sampleHex + "  ./dist/tool.zip\n",
			want: map[string]string{"tool.zip": "sha256:" + sampleHex},
		},
		{
			name: "conflicting duplicates are dropped",
			body: sampleHex + "  a/tool.zip\n" + other + "  b/tool.zip\n" + sampleHex + "  tool.zip\n",
			want: map[string]string{},
		},
		{
			name: "blank, malformed and nameless lines are skipped",
			body: "\n# comment\nnot-a-digest tool.zip\n" + sampleHex + "\n" + sampleHex + " *\n",
			want: map[string]string{},
		},
		{
			name:   "single asset file with a bare digest",
			body:   sampleHex + "\n",
			target: "tool.appimage",
			want:   map[string]string{"tool.appimage": "sha256:" + sampleHex},
		},
		{
			name:   "single asset file naming another file",
			body:   sampleHex + "  other.appimage\n",
			target: "tool.appimage",
			want:   map[string]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sums := newChecksumSet()

			parseChecksums(sums, []byte(tc.body), tc.target)

			assert.Equal(t, tc.want, sums.digests)
		})
	}
}

func TestChecksumSet_Lookup(t *testing.T) {
	sums := newChecksumSet()
	sums.add("a.zip", "sha256:a")
	sums.add("b.zip", "sha256:b")
	sums.add("same.zip", "sha256:a")

	testCases := []struct {
		name  string
		names []string
		want  string
	}{
		{name: "single hit", names: []string{"nope", "a.zip"}, want: "sha256:a"},
		{name: "agreeing hits", names: []string{"a.zip", "same.zip"}, want: "sha256:a"},
		{name: "disagreeing hits", names: []string{"a.zip", "b.zip"}},
		{name: "no hit", names: []string{"nope"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sums.lookup(tc.names))
		})
	}
}
