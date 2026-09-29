package media

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

const (
	githubRawTemplate  = "https://raw.githubusercontent.com/owner/repo/{branch}/{file}"
	githubBlobTemplate = "https://github.com/owner/repo/blob/{branch}/{file}"
	testRef            = "v1.0.0"
)

var errMissing = errors.New("missing")

type stubHost struct {
	hosts.Host
	files        map[string][]byte
	images       map[string][]byte
	rawTemplate  string
	blobTemplate string
	noTemplates  bool
}

func (s *stubHost) templates() (string, string) {
	if s.rawTemplate == "" {
		return githubRawTemplate, githubBlobTemplate
	}
	return s.rawTemplate, s.blobTemplate
}

func (s *stubHost) RawFileURL(
	_ domain.Namespace,
	ref string,
	file string,
) (string, error) {
	raw, _ := s.templates()
	return fill(raw, ref, file, s.noTemplates)
}

func (s *stubHost) BlobFileURL(
	_ domain.Namespace,
	ref string,
	file string,
) (string, error) {
	_, blob := s.templates()
	return fill(blob, ref, file, s.noTemplates)
}

func fill(
	template string,
	ref string,
	file string,
	missing bool,
) (string, error) {
	if missing {
		return "", errMissing
	}
	return strings.NewReplacer("{branch}", ref, "{file}", file).Replace(template), nil
}

func (s *stubHost) fetch(
	_ context.Context,
	url string,
) ([]byte, error) {
	if data, ok := s.images[url]; ok {
		return data, nil
	}
	raw, _ := s.templates()
	path, ok := strings.CutPrefix(url, strings.NewReplacer("{branch}", testRef, "{file}", "").Replace(raw))
	if !ok {
		return nil, errMissing
	}
	data, ok := s.files[path]
	if !ok {
		return nil, errMissing
	}
	return data, nil
}

func resolveAll(
	ctx context.Context,
	host *stubHost,
	ns domain.Namespace,
	socialImage string,
	readmeRaw []byte,
) domain.ArrowMedia {
	probed := ProbeIcon(ctx, host.fetch, host, ns, testRef)
	return Resolve(ctx, host.fetch, host, ns, testRef, socialImage, readmeRaw, probed)
}

const (
	testNS          = domain.Namespace("github.com/owner/repo@v1.0.0")
	githubRawPrefix = "https://raw.githubusercontent.com/owner/repo/v1.0.0/"
)

func TestResolve_Cases(
	t *testing.T,
) {
	const social = "https://images.example.test/social.png"
	square := encodePNG(t, 300, 300)
	wide := encodePNG(t, 1200, 400)
	testCases := []struct {
		name       string
		files      map[string][]byte
		social     map[string][]byte
		readme     string
		wantIcon   string
		wantBanner string
	}{
		{name: "probe path in priority order", files: map[string][]byte{"src-tauri/icons/icon.png": encodePNG(t, 256, 256)}, wantIcon: githubRawPrefix + "src-tauri/icons/icon.png"},
		{name: "probed svg of any size", files: map[string][]byte{"logo.svg": []byte(`<svg width="8" height="8"><path/></svg>`)}, wantIcon: githubRawPrefix + "logo.svg"},
		{name: "too small probe falls to the next", files: map[string][]byte{"src-tauri/icons/icon.png": encodePNG(t, 64, 64), "build/icon.png": encodePNG(t, 256, 256)}, wantIcon: githubRawPrefix + "build/icon.png"},
		{name: "readme square image", files: map[string][]byte{"icon-art.png": square}, readme: "![Icon](icon-art.png)\n", wantIcon: githubRawPrefix + "icon-art.png"},
		{name: "icon skips badges", files: map[string][]byte{"icon-art.png": square}, readme: "![Build](https://img.shields.io/x.svg)\n![Icon](icon-art.png)\n", wantIcon: githubRawPrefix + "icon-art.png"},
		{name: "icon ignores images past the line limit", files: map[string][]byte{"icon-art.png": square}, readme: strings.Repeat("prose line\n", 61) + "![Icon](icon-art.png)\n"},
		{name: "icon skips a missing file", files: map[string][]byte{"icon-art.png": square}, readme: "![Icon](missing.png)\n![Icon](icon-art.png)\n", wantIcon: githubRawPrefix + "icon-art.png"},
		{name: "icon skips unsniffable data", files: map[string][]byte{"garbage.png": []byte("not an image"), "icon-art.png": square}, readme: "![Icon](garbage.png)\n![Icon](icon-art.png)\n", wantIcon: githubRawPrefix + "icon-art.png"},
		{name: "banner shaped social image", social: map[string][]byte{social: encodePNG(t, 1280, 640)}, wantBanner: social},
		{name: "square social image falls back to the readme", files: map[string][]byte{"shot.png": wide}, social: map[string][]byte{social: square}, readme: "![s](shot.png)\n", wantBanner: githubRawPrefix + "shot.png"},
		{name: "unsniffable social image", social: map[string][]byte{social: []byte("not an image")}},
		{name: "unreachable social image"},
		{name: "wide readme banner", files: map[string][]byte{"shot.png": wide}, readme: "![Screenshot](shot.png)\n", wantBanner: githubRawPrefix + "shot.png"},
		{name: "screenshot shaped banner", files: map[string][]byte{"shot.png": encodePNG(t, 640, 360)}, readme: "![Screenshot](shot.png)\n", wantBanner: githubRawPrefix + "shot.png"},
		{name: "no banner when nothing qualifies", files: map[string][]byte{"shot.png": encodePNG(t, 100, 100)}, readme: "![Screenshot](shot.png)\n", wantIcon: githubRawPrefix + "shot.png"},
		{name: "banner skips badges", files: map[string][]byte{"shot.png": wide}, readme: "![Build](https://img.shields.io/x.svg)\n![Screenshot](shot.png)\n", wantBanner: githubRawPrefix + "shot.png"},
		{name: "external image is not resolvable", readme: "![Screenshot](https://someone-elses-cdn.example/shot.png)\n"},
		{name: "protocol relative image is not resolvable", readme: "![Screenshot](//cdn.example.com/img.png)\n"},
		{name: "same repo raw url at another ref", files: map[string][]byte{"doc/logo.png": wide}, readme: "![s](https://raw.githubusercontent.com/owner/repo/main/doc/logo.png)\n", wantBanner: githubRawPrefix + "doc/logo.png"},
		{name: "malformed same repo raw url", readme: "![s](https://raw.githubusercontent.com/owner/repo/main)\n"},
		{name: "other repo raw url", files: map[string][]byte{"x.png": wide}, readme: "![s](https://raw.githubusercontent.com/other/repo/main/x.png)\n"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host := &stubHost{files: tc.files, images: tc.social}

			media := resolveAll(context.Background(), host, testNS, social, []byte(tc.readme))

			assert.Equal(t, tc.wantIcon, media.Icon)
			assert.Equal(t, tc.wantBanner, media.Banner)
		})
	}
}
