package forge_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/forge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
)

func TestLocalFile(t *testing.T) {
	testCases := []struct {
		name   string
		url    string
		digest string
		want   string
		ok     bool
	}{
		{name: "plain name", url: "https://example.test/v1/tool.tar.gz", digest: digest, want: "tool.tar.gz", ok: true},
		{name: "escaped name is unescaped", url: "https://example.test/v1/tool%2Bx_y-1.zip", digest: digest, want: "tool+x_y-1.zip", ok: true},
		{name: "space", url: "https://example.test/v1/tool%20y.zip", digest: digest, ok: false},
		{name: "non ascii letter", url: "https://example.test/v1/t%C3%B6ol.zip", digest: digest, ok: false},
		{name: "shell metacharacter", url: "https://example.test/v1/tool%3Bx.zip", digest: digest, ok: false},
		{name: "query is ignored", url: "https://example.test/v1/tool.zip?sig=a/b", digest: digest, want: "tool.zip", ok: true},
		{name: "empty digest", url: "https://example.test/v1/tool.zip", digest: "", ok: false},
		{name: "unparseable url", url: "https://example.test/%zz", digest: digest, ok: false},
		{name: "unparseable host", url: "http://[::1", digest: digest, ok: false},
		{name: "empty url", url: "", digest: digest, ok: false},
		{name: "http scheme", url: "http://127.0.0.1:8080/v1/tool.tar.gz", digest: digest, want: "tool.tar.gz", ok: true},
		{name: "file scheme", url: "file:///etc/tool.tar.gz", digest: digest, ok: false},
		{name: "ftp scheme", url: "ftp://example.test/v1/tool.tar.gz", digest: digest, ok: false},
		{name: "data scheme", url: "data:application/octet-stream;base64,AAAA", digest: digest, ok: false},
		{name: "empty scheme", url: "//example.test/v1/tool.tar.gz", digest: digest, ok: false},
		{name: "relative path", url: "v1/tool.tar.gz", digest: digest, ok: false},
		{name: "uppercase https scheme", url: "HTTPS://example.test/v1/tool.tar.gz", digest: digest, want: "tool.tar.gz", ok: true},
		{name: "root path", url: "https://example.test/", digest: digest, ok: false},
		{name: "dot", url: "https://example.test/a/.", digest: digest, ok: false},
		{name: "dot dot", url: "https://example.test/a/..", digest: digest, ok: false},
		{name: "encoded slash", url: "https://example.test/a%2Fb", digest: digest, ok: false},
		{name: "encoded backslash", url: "https://example.test/a%5Cb", digest: digest, ok: false},
		{name: "dollar", url: "https://example.test/$x", digest: digest, ok: false},
		{name: "open brace", url: "https://example.test/%7Bx", digest: digest, ok: false},
		{name: "close brace", url: "https://example.test/x%7D", digest: digest, ok: false},
		{name: "newline", url: "https://example.test/x%0Ay", digest: digest, ok: false},
		{name: "tab", url: "https://example.test/x%09y", digest: digest, ok: false},
		{name: "hidden file", url: "https://example.test/.tool", digest: digest, ok: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pick := picker.Pick{Asset: domain.ReleaseAsset{Name: "ignored", URL: tc.url, Digest: tc.digest}}

			got, ok := forge.LocalFile(pick)

			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}
