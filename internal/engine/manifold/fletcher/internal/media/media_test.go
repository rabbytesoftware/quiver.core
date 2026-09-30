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
	githubRawTemplate = "https://raw.githubusercontent.com/owner/repo/{branch}/{file}"
	testRef           = "v1.0.0"
	testBranch        = "main"
	testNS            = domain.Namespace("github.com/owner/repo@v1.0.0")
	githubRawPrefix   = "https://raw.githubusercontent.com/owner/repo/v1.0.0/"
	githubMainPrefix  = "https://raw.githubusercontent.com/owner/repo/main/"
	squareSVG         = `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"></svg>`
)

var errMissing = errors.New("missing")

type stubHost struct {
	hosts.Host
	tagged      map[string][]byte
	branch      map[string][]byte
	images      map[string][]byte
	rawTemplate string
	noTemplates bool
}

func (s *stubHost) RawFileURL(
	_ domain.Namespace,
	ref string,
	file string,
) (string, error) {
	if s.noTemplates {
		return "", errMissing
	}
	template := s.rawTemplate
	if template == "" {
		template = githubRawTemplate
	}
	return strings.NewReplacer("{branch}", ref, "{file}", file).Replace(template), nil
}

func (s *stubHost) DefaultBranches() []string {
	return []string{testBranch}
}

func (s *stubHost) fetch(
	_ context.Context,
	url string,
) ([]byte, error) {
	if data, ok := s.images[url]; ok {
		return data, nil
	}
	template := s.rawTemplate
	if template == "" {
		template = githubRawTemplate
	}
	for ref, files := range map[string]map[string][]byte{testRef: s.tagged, testBranch: s.branch} {
		prefix := strings.NewReplacer("{branch}", ref, "{file}", "").Replace(template)
		path, ok := strings.CutPrefix(url, prefix)
		if !ok {
			continue
		}
		if data, ok := files[path]; ok {
			return data, nil
		}
	}
	return nil, errMissing
}

func TestResolve_Cases(
	t *testing.T,
) {
	const (
		social = "https://images.example.test/social.png"
		avatar = "https://avatars.example.test/u/1"
	)
	square := encodePNG(t, 300, 300)
	testCases := []struct {
		name       string
		social     map[string][]byte
		repoIcon   string
		avatar     string
		wantIcon   string
		wantBanner string
	}{
		{name: "repo icon wins over the avatar", repoIcon: "https://x.test/icon.png", avatar: avatar, wantIcon: "https://x.test/icon.png"},
		{name: "avatar backs up a missing repo icon", avatar: avatar, wantIcon: avatar},
		{name: "no icon at all"},
		{name: "banner shaped social image", social: map[string][]byte{social: encodePNG(t, 1280, 640)}, wantBanner: social},
		{name: "square social image is no banner", social: map[string][]byte{social: square}},
		{name: "unsniffable social image", social: map[string][]byte{social: []byte("not an image")}},
		{name: "unreachable social image"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			host := &stubHost{images: tc.social}

			got := Resolve(context.Background(), host.fetch, social, tc.repoIcon, tc.avatar)

			assert.Equal(t, tc.wantIcon, got.Icon)
			assert.Equal(t, tc.wantBanner, got.Banner)
		})
	}
}

func TestResolve_EmptySocialImageIsNoBanner(
	t *testing.T,
) {
	host := &stubHost{}

	got := Resolve(context.Background(), host.fetch, "", "", "")

	assert.Empty(t, got.Banner)
}

func TestIsBannerShaped_Cases(
	t *testing.T,
) {
	testCases := []struct {
		name string
		dim  dimensions
		want bool
	}{
		{name: "very wide", dim: dimensions{Width: 600, Height: 100}, want: true},
		{name: "screenshot", dim: dimensions{Width: 640, Height: 360}, want: true},
		{name: "small landscape", dim: dimensions{Width: 300, Height: 200}},
		{name: "square", dim: dimensions{Width: 500, Height: 500}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isBannerShaped(tc.dim))
		})
	}
}
