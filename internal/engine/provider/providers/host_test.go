package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// ─── raw file URLs ───────────────────────────────────────────────────────────

// Each host serves raw files at a shape of its own, which is exactly the
// knowledge that belongs here and not in the manifest resolver.
func TestHost_RawFileURL_PerHostShape(t *testing.T) {
	testCases := []struct {
		name   string
		build  func(Config) Provider
		host   string
		rawURL string
		ns     domain.Namespace
		want   string
	}{
		{
			name:   "github",
			build:  NewGitHub,
			host:   "github.com",
			rawURL: "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
			ns:     domain.Namespace("github.com/myuser/myrepo"),
			want:   "https://raw.githubusercontent.com/myuser/myrepo/main/arrow.yaml",
		},
		{
			name:   "gitlab",
			build:  NewGitLab,
			host:   "gitlab.com",
			rawURL: "https://gitlab.com/{user}/{repo}/-/raw/{branch}/{file}",
			ns:     domain.Namespace("gitlab.com/myuser/myrepo"),
			want:   "https://gitlab.com/myuser/myrepo/-/raw/main/arrow.yaml",
		},
		{
			name:   "bitbucket",
			build:  NewBitbucket,
			host:   "bitbucket.org",
			rawURL: "https://bitbucket.org/{user}/{repo}/raw/{branch}/{file}",
			ns:     domain.Namespace("bitbucket.org/myuser/myrepo"),
			want:   "https://bitbucket.org/myuser/myrepo/raw/main/arrow.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.build(Config{Host: tc.host, RawURL: tc.rawURL})

			got, err := p.RawFileURL(tc.ns, "main", "arrow.yaml")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// A ref names a revision, not a repository, so it never reaches the URL except
// where the caller put it — in the branch position.
func TestHost_RawFileURL_IgnoresTheRefOnTheNamespace(t *testing.T) {
	p := NewGitHub(Config{
		Host:   "github.com",
		RawURL: "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
	})

	got, err := p.RawFileURL(domain.Namespace("github.com/u/r@v1.2.3"), "v1.2.3", "ARROW.md")
	require.NoError(t, err)
	assert.Equal(t, "https://raw.githubusercontent.com/u/r/v1.2.3/ARROW.md", got)
}

// A quiver-hosted namespace carries a fourth segment naming the arrow inside
// the repository, which addresses a file and never the repository itself.
func TestHost_RawFileURL_QuiverHostedNamespaceUsesTheRepository(t *testing.T) {
	p := NewGitHub(Config{
		Host:   "github.com",
		RawURL: "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
	})

	got, err := p.RawFileURL(domain.Namespace("github.com/char2cs/gaming.quiver/cs2"), "main", "cs2.md")
	require.NoError(t, err)
	assert.Equal(t, "https://raw.githubusercontent.com/char2cs/gaming.quiver/main/cs2.md", got)
}

func TestHost_RawFileURL_InvalidNamespace_ReturnsError(t *testing.T) {
	p := NewGitHub(Config{Host: "github.com", RawURL: "https://raw/{user}/{repo}/{branch}/{file}"})

	_, err := p.RawFileURL(domain.Namespace("github.com/only-two"), "main", "arrow.yaml")
	require.Error(t, err)
}

func TestHost_RawFileURL_HostWithoutARawURL_ReturnsErrNoRawURL(t *testing.T) {
	p := NewBitbucket(Config{Host: "example.com"})

	_, err := p.RawFileURL(domain.Namespace("example.com/u/r"), "main", "arrow.yaml")
	assert.ErrorIs(t, err, ErrNoRawURL)
}

func TestHost_DefaultBranches_AreTheConfiguredOnes(t *testing.T) {
	p := NewGitLab(Config{Host: "gitlab.com", DefaultBranches: []string{"main", "master"}})
	assert.Equal(t, []string{"main", "master"}, p.DefaultBranches())
}

// ─── search capability ───────────────────────────────────────────────────────

func TestHost_CanSearch(t *testing.T) {
	testCases := []struct {
		name  string
		build func(Config) Provider
		cfg   Config
		want  bool
	}{
		{
			name:  "github with a search url",
			build: NewGitHub,
			cfg:   Config{Host: "github.com", SearchURL: "https://api.github.com/search?q={query}"},
			want:  true,
		},
		{
			name:  "github without one",
			build: NewGitHub,
			cfg:   Config{Host: "github.com"},
		},
		{
			name:  "gitlab with a search url",
			build: NewGitLab,
			cfg:   Config{Host: "gitlab.com", SearchURL: "https://gitlab.com/api?search={query}"},
			want:  true,
		},
		{
			name:  "gitlab without one",
			build: NewGitLab,
			cfg:   Config{Host: "gitlab.com"},
		},
		{
			name:  "bitbucket, which has no search api at all",
			build: NewBitbucket,
			cfg:   Config{Host: "bitbucket.org", SearchURL: "https://bitbucket.org/ignored"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.build(tc.cfg).CanSearch())
		})
	}
}

// A host that cannot search says so rather than returning an empty result set,
// which would read as "nothing published there".
func TestHost_Search_UnsupportedHostRefuses(t *testing.T) {
	p := NewBitbucket(Config{Host: "bitbucket.org"})

	got, err := p.Search(context.Background(), SearchRequest{Text: "browser"})
	assert.ErrorIs(t, err, ErrSearchUnsupported)
	assert.Contains(t, err.Error(), "bitbucket.org")
	assert.Nil(t, got)
}

func TestHost_Host_ReturnsTheConfiguredHost(t *testing.T) {
	assert.Equal(t, "bitbucket.org", NewBitbucket(Config{Host: "bitbucket.org"}).Host())
}

// A host with a search dialect but no endpoint configured refuses in the same
// words as a host that has no dialect at all.
func TestHost_Search_ConfiguredWithoutAnEndpointRefuses(t *testing.T) {
	testCases := []struct {
		name  string
		build func(Config) Provider
		host  string
	}{
		{name: "github", build: NewGitHub, host: "github.com"},
		{name: "gitlab", build: NewGitLab, host: "gitlab.com"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.build(Config{Host: tc.host})

			_, err := p.Search(context.Background(), SearchRequest{Text: "x"})
			assert.ErrorIs(t, err, ErrSearchUnsupported)
		})
	}
}
