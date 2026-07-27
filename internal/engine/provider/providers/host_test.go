package providers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// redirectTo answers every request with one 302 pointing at location, which is
// the whole of what a release permalink says.
func redirectTo(location string) DoFunc {
	headers := http.Header{}
	headers.Set("Location", location)

	return func(_ context.Context, _ fns.Request) (fns.Response, error) {
		return fns.Response{Status: http.StatusFound, Headers: headers}, nil
	}
}

func answerStatus(status int) DoFunc {
	return func(_ context.Context, _ fns.Request) (fns.Response, error) {
		return fns.Response{Status: status, Headers: http.Header{}}, nil
	}
}

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

// ─── latest release ──────────────────────────────────────────────────────────

// The permalink redirects to the release page, and where the ref sits in that
// URL is the host's business: GitHub puts it after "/releases/tag/", GitLab
// straight after "/-/releases/". Reading GitHub's shape on a GitLab answer
// would miss every release GitLab publishes.
func TestHost_LatestRelease_PerHostRedirectShape(t *testing.T) {
	testCases := []struct {
		name       string
		build      func(Config) Provider
		host       string
		releaseURL string
		location   string
		want       string
	}{
		{
			name:       "github tag permalink",
			build:      NewGitHub,
			host:       "github.com",
			releaseURL: "https://github.com/{user}/{repo}/releases/latest",
			location:   "https://github.com/cli/cli/releases/tag/v2.96.0",
			want:       "v2.96.0",
		},
		{
			name:       "github relative redirect",
			build:      NewGitHub,
			host:       "github.com",
			releaseURL: "https://github.com/{user}/{repo}/releases/latest",
			location:   "/cli/cli/releases/tag/v2.96.0",
			want:       "v2.96.0",
		},
		{
			name:       "gitlab permalink",
			build:      NewGitLab,
			host:       "gitlab.com",
			releaseURL: "https://gitlab.com/{user}/{repo}/-/releases/permalink/latest",
			location:   "https://gitlab.com/acme/tool/-/releases/v1.4.0",
			want:       "v1.4.0",
		},
		{
			name:       "gitlab relative redirect",
			build:      NewGitLab,
			host:       "gitlab.com",
			releaseURL: "https://gitlab.com/{user}/{repo}/-/releases/permalink/latest",
			location:   "/acme/tool/-/releases/v1.4.0",
			want:       "v1.4.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.build(Config{
				Host:             tc.host,
				LatestReleaseURL: tc.releaseURL,
				Do:               redirectTo(tc.location),
			})

			got, err := p.LatestRelease(context.Background(), domain.Namespace(tc.host+"/acme/tool"))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// A redirect to the release index carries no ref, which is how a host says it
// has published no stable release.
func TestHost_LatestRelease_RedirectToTheReleaseIndexMisses(t *testing.T) {
	testCases := []struct {
		name       string
		build      func(Config) Provider
		host       string
		releaseURL string
		location   string
	}{
		{
			name:       "github",
			build:      NewGitHub,
			host:       "github.com",
			releaseURL: "https://github.com/{user}/{repo}/releases/latest",
			location:   "https://github.com/acme/tool/releases",
		},
		{
			name:       "gitlab",
			build:      NewGitLab,
			host:       "gitlab.com",
			releaseURL: "https://gitlab.com/{user}/{repo}/-/releases/permalink/latest",
			location:   "https://gitlab.com/acme/tool/-/releases",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.build(Config{
				Host:             tc.host,
				LatestReleaseURL: tc.releaseURL,
				Do:               redirectTo(tc.location),
			})

			_, err := p.LatestRelease(context.Background(), domain.Namespace(tc.host+"/acme/tool"))
			assert.ErrorIs(t, err, ErrNoLatestRelease)
		})
	}
}

func TestHost_LatestRelease_LocationVariants(t *testing.T) {
	testCases := []struct {
		name     string
		location string
		want     string
		wantMiss bool
	}{
		{
			name:     "escaped slash in ref",
			location: "https://github.com/u/r/releases/tag/release%2Fv1.2",
			want:     "release/v1.2",
		},
		{
			name:     "query string is dropped",
			location: "https://github.com/u/r/releases/tag/v1.0.0?foo=bar",
			want:     "v1.0.0",
		},
		{
			name:     "fragment is dropped",
			location: "https://github.com/u/r/releases/tag/v1.0.0#notes",
			want:     "v1.0.0",
		},
		{
			name:     "trailing slash is trimmed",
			location: "https://github.com/u/r/releases/tag/v1.0.0/",
			want:     "v1.0.0",
		},
		{name: "empty location is a miss", location: "", wantMiss: true},
		{
			name:     "empty ref after the marker is a miss",
			location: "https://github.com/u/r/releases/tag/",
			wantMiss: true,
		},
		{
			name:     "only slashes after the marker is a miss",
			location: "https://github.com/u/r/releases/tag///",
			wantMiss: true,
		},
		{
			name:     "invalid escape is a miss",
			location: "https://github.com/u/r/releases/tag/v1%zz",
			wantMiss: true,
		},
		{
			name:     "ref escaping to empty is a miss",
			location: "https://github.com/u/r/releases/tag/%2F",
			wantMiss: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewGitHub(Config{
				Host:             "github.com",
				LatestReleaseURL: "https://github.com/{user}/{repo}/releases/latest",
				Do:               redirectTo(tc.location),
			})

			got, err := p.LatestRelease(context.Background(), domain.Namespace("github.com/u/r"))
			if tc.wantMiss {
				assert.ErrorIs(t, err, ErrNoLatestRelease)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// A host that publishes no releases misses cleanly rather than requesting a
// page that does not exist.
func TestHost_LatestRelease_HostWithoutAPermalinkMissesWithoutRequesting(t *testing.T) {
	var calls int
	counted := func(ctx context.Context, req fns.Request) (fns.Response, error) {
		calls++
		return redirectTo("/u/r/releases/tag/v1.0.0")(ctx, req)
	}

	p := NewBitbucket(Config{Host: "bitbucket.org", Do: counted})

	_, err := p.LatestRelease(context.Background(), domain.Namespace("bitbucket.org/u/r"))
	assert.ErrorIs(t, err, ErrNoLatestRelease)
	assert.Zero(t, calls, "a host with no release permalink has nothing to request")
}

func TestHost_LatestRelease_NonRedirectStatusMisses(t *testing.T) {
	testCases := []struct {
		name   string
		status int
	}{
		{name: "ok", status: http.StatusOK},
		{name: "not found", status: http.StatusNotFound},
		{name: "server error", status: http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewGitHub(Config{
				Host:             "github.com",
				LatestReleaseURL: "https://github.com/{user}/{repo}/releases/latest",
				Do:               answerStatus(tc.status),
			})

			_, err := p.LatestRelease(context.Background(), domain.Namespace("github.com/u/r"))
			assert.ErrorIs(t, err, ErrNoLatestRelease)
		})
	}
}

func TestHost_LatestRelease_TransportFailureMisses(t *testing.T) {
	failing := func(_ context.Context, _ fns.Request) (fns.Response, error) {
		return fns.Response{}, errors.New("dial tcp: connection refused")
	}

	p := NewGitHub(Config{
		Host:             "github.com",
		LatestReleaseURL: "https://github.com/{user}/{repo}/releases/latest",
		Do:               failing,
	})

	_, err := p.LatestRelease(context.Background(), domain.Namespace("github.com/u/r"))
	assert.ErrorIs(t, err, ErrNoLatestRelease)
}

func TestHost_LatestRelease_InvalidNamespaceMisses(t *testing.T) {
	p := NewGitHub(Config{
		Host:             "github.com",
		LatestReleaseURL: "https://github.com/{user}/{repo}/releases/latest",
		Do:               redirectTo("/u/r/releases/tag/v1.0.0"),
	})

	_, err := p.LatestRelease(context.Background(), domain.Namespace("github.com/only-two"))
	assert.ErrorIs(t, err, ErrNoLatestRelease)
}

// The permalink is asked about a repository, so a namespace's ref never reaches
// the URL: the release the host names is the answer being looked for.
func TestHost_LatestRelease_IgnoresTheRefOnTheNamespace(t *testing.T) {
	var requested string
	recording := func(ctx context.Context, req fns.Request) (fns.Response, error) {
		requested = req.URL
		return redirectTo("/u/r/releases/tag/v3.0.0")(ctx, req)
	}

	p := NewGitHub(Config{
		Host:             "github.com",
		LatestReleaseURL: "https://github.com/{user}/{repo}/releases/latest",
		Do:               recording,
	})

	got, err := p.LatestRelease(context.Background(), domain.Namespace("github.com/u/r@nightly"))
	require.NoError(t, err)
	assert.Equal(t, "v3.0.0", got)
	assert.Equal(t, "https://github.com/u/r/releases/latest", requested)
}

func TestHost_LatestRelease_CancelledContextMisses(t *testing.T) {
	p := NewGitHub(Config{
		Host:             "github.com",
		LatestReleaseURL: "https://github.com/{user}/{repo}/releases/latest",
		Timeout:          time.Second,
		Do: func(ctx context.Context, _ fns.Request) (fns.Response, error) {
			return fns.Response{}, ctx.Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.LatestRelease(ctx, domain.Namespace("github.com/u/r"))
	assert.ErrorIs(t, err, ErrNoLatestRelease)
}

// The default transport is the one production uses. A nanosecond deadline
// expires before any socket is opened, so this exercises it without a network.
func TestHost_LatestRelease_NilDo_UsesTheDefaultTransport(t *testing.T) {
	p := NewGitHub(Config{
		Host:             "github.com",
		LatestReleaseURL: "https://github.com/{user}/{repo}/releases/latest",
		Timeout:          time.Nanosecond,
	})

	_, err := p.LatestRelease(context.Background(), domain.Namespace("github.com/u/r"))
	assert.ErrorIs(t, err, ErrNoLatestRelease)
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
