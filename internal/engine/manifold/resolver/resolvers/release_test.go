package resolvers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// newReleaseServer serves a single latest-release permalink whose response is
// produced by handle. The returned platform table points github.com at it.
func newReleaseServer(
	t *testing.T,
	handle http.HandlerFunc,
) (metadata.Platforms, func()) {
	t.Helper()

	srv := httptest.NewServer(handle)
	platforms := metadata.Platforms{
		"github.com": {
			LatestReleaseURL: srv.URL + "/{user}/{repo}/releases/latest",
		},
	}

	return platforms, srv.Close
}

func redirectTo(location string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", location)
		w.WriteHeader(http.StatusFound)
	}
}

// ─── refFromLocation ──────────────────────────────────────────────────────────

func TestRefFromLocation(t *testing.T) {
	testCases := []struct {
		name     string
		location string
		want     string
		wantOK   bool
	}{
		{
			name:     "absolute tag permalink",
			location: "https://github.com/cli/cli/releases/tag/v2.96.0",
			want:     "v2.96.0",
			wantOK:   true,
		},
		{
			name:     "relative tag permalink",
			location: "/cli/cli/releases/tag/v2.96.0",
			want:     "v2.96.0",
			wantOK:   true,
		},
		{
			name:     "escaped slash in ref",
			location: "https://github.com/u/r/releases/tag/release%2Fv1.2",
			want:     "release/v1.2",
			wantOK:   true,
		},
		{
			name:     "query string is dropped",
			location: "https://github.com/u/r/releases/tag/v1.0.0?foo=bar",
			want:     "v1.0.0",
			wantOK:   true,
		},
		{
			name:     "fragment is dropped",
			location: "https://github.com/u/r/releases/tag/v1.0.0#notes",
			want:     "v1.0.0",
			wantOK:   true,
		},
		{
			name:     "trailing slash is trimmed",
			location: "https://github.com/u/r/releases/tag/v1.0.0/",
			want:     "v1.0.0",
			wantOK:   true,
		},
		{
			name:     "release index is a miss",
			location: "https://github.com/u/r/releases",
			wantOK:   false,
		},
		{
			name:     "empty location is a miss",
			location: "",
			wantOK:   false,
		},
		{
			name:     "empty ref after marker is a miss",
			location: "https://github.com/u/r/releases/tag/",
			wantOK:   false,
		},
		{
			name:     "only slashes after marker is a miss",
			location: "https://github.com/u/r/releases/tag///",
			wantOK:   false,
		},
		{
			name:     "invalid escape is a miss",
			location: "https://github.com/u/r/releases/tag/v1%zz",
			wantOK:   false,
		},
		{
			name:     "ref escaping to empty is a miss",
			location: "https://github.com/u/r/releases/tag/%2F",
			wantOK:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := refFromLocation(tc.location)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("ref = %q, want %q", got, tc.want)
			}
		})
	}
}

// ─── buildReleaseURL ──────────────────────────────────────────────────────────

func TestBuildReleaseURL_ReplacesPlaceholders(t *testing.T) {
	got := buildReleaseURL("https://github.com/{user}/{repo}/releases/latest", "cli", "cli")
	want := "https://github.com/cli/cli/releases/latest"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ─── NewReleaseResolver ───────────────────────────────────────────────────────

func TestNewReleaseResolver_ZeroTimeoutUsesDefault(t *testing.T) {
	r, ok := NewReleaseResolver(metadata.Platforms{}, 0).(*releaseResolver)
	if !ok {
		t.Fatal("expected *releaseResolver")
	}
	if r.timeout != defaultReleaseTimeout {
		t.Errorf("timeout = %v, want %v", r.timeout, defaultReleaseTimeout)
	}
}

func TestNewReleaseResolver_KeepsExplicitTimeout(t *testing.T) {
	r, ok := NewReleaseResolver(metadata.Platforms{}, 2*time.Second).(*releaseResolver)
	if !ok {
		t.Fatal("expected *releaseResolver")
	}
	if r.timeout != 2*time.Second {
		t.Errorf("timeout = %v, want 2s", r.timeout)
	}
}

// ─── Latest ───────────────────────────────────────────────────────────────────

func TestReleaseResolver_Latest_TagRedirectHits(t *testing.T) {
	platforms, closeSrv := newReleaseServer(
		t,
		redirectTo("https://github.com/cli/cli/releases/tag/v2.96.0"),
	)
	defer closeSrv()

	got, err := NewReleaseResolver(platforms, time.Second).
		Latest(context.Background(), domain.Namespace("github.com/cli/cli"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v2.96.0" {
		t.Errorf("ref = %q, want %q", got, "v2.96.0")
	}
}

func TestReleaseResolver_Latest_IgnoresRefOnNamespace(t *testing.T) {
	var requested string
	platforms, closeSrv := newReleaseServer(t, func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		w.Header().Set("Location", "/u/r/releases/tag/v3.0.0")
		w.WriteHeader(http.StatusFound)
	})
	defer closeSrv()

	got, err := NewReleaseResolver(platforms, time.Second).
		Latest(context.Background(), domain.Namespace("github.com/u/r@nightly"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v3.0.0" {
		t.Errorf("ref = %q, want %q", got, "v3.0.0")
	}
	if requested != "/u/r/releases/latest" {
		t.Errorf("requested %q, want %q", requested, "/u/r/releases/latest")
	}
}

func TestReleaseResolver_Latest_RedirectToReleaseIndexMisses(t *testing.T) {
	platforms, closeSrv := newReleaseServer(t, redirectTo("https://github.com/u/r/releases"))
	defer closeSrv()

	_, err := NewReleaseResolver(platforms, time.Second).
		Latest(context.Background(), domain.Namespace("github.com/u/r"))
	if !errors.Is(err, ErrNoLatestRelease) {
		t.Fatalf("expected ErrNoLatestRelease, got %v", err)
	}
}

func TestReleaseResolver_Latest_NonRedirectStatusMisses(t *testing.T) {
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
			platforms, closeSrv := newReleaseServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			})
			defer closeSrv()

			_, err := NewReleaseResolver(platforms, time.Second).
				Latest(context.Background(), domain.Namespace("github.com/u/r"))
			if !errors.Is(err, ErrNoLatestRelease) {
				t.Fatalf("expected ErrNoLatestRelease, got %v", err)
			}
		})
	}
}

func TestReleaseResolver_Latest_TransportFailureMisses(t *testing.T) {
	platforms, closeSrv := newReleaseServer(t, redirectTo("/u/r/releases/tag/v1.0.0"))
	closeSrv() // the server is down before the request is issued

	_, err := NewReleaseResolver(platforms, time.Second).
		Latest(context.Background(), domain.Namespace("github.com/u/r"))
	if !errors.Is(err, ErrNoLatestRelease) {
		t.Fatalf("expected ErrNoLatestRelease, got %v", err)
	}
}

func TestReleaseResolver_Latest_UnknownPlatformMisses(t *testing.T) {
	_, err := NewReleaseResolver(metadata.Platforms{}, time.Second).
		Latest(context.Background(), domain.Namespace("example.com/u/r"))
	if !errors.Is(err, ErrNoLatestRelease) {
		t.Fatalf("expected ErrNoLatestRelease, got %v", err)
	}
	if !strings.Contains(err.Error(), "example.com") {
		t.Errorf("error %q should name the platform", err)
	}
}

func TestReleaseResolver_Latest_PlatformWithoutTemplateMisses(t *testing.T) {
	platforms := metadata.Platforms{
		"bitbucket.org": {RawURL: "https://bitbucket.org/{user}/{repo}/raw/{branch}/{file}"},
	}

	_, err := NewReleaseResolver(platforms, time.Second).
		Latest(context.Background(), domain.Namespace("bitbucket.org/u/r"))
	if !errors.Is(err, ErrNoLatestRelease) {
		t.Fatalf("expected ErrNoLatestRelease, got %v", err)
	}
}

func TestReleaseResolver_Latest_InvalidNamespaceMisses(t *testing.T) {
	_, err := NewReleaseResolver(metadata.Platforms{}, time.Second).
		Latest(context.Background(), domain.Namespace("github.com/only-two"))
	if !errors.Is(err, ErrNoLatestRelease) {
		t.Fatalf("expected ErrNoLatestRelease, got %v", err)
	}
}

func TestReleaseResolver_Latest_CancelledContextMisses(t *testing.T) {
	platforms, closeSrv := newReleaseServer(t, redirectTo("/u/r/releases/tag/v1.0.0"))
	defer closeSrv()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewReleaseResolver(platforms, time.Second).
		Latest(ctx, domain.Namespace("github.com/u/r"))
	if !errors.Is(err, ErrNoLatestRelease) {
		t.Fatalf("expected ErrNoLatestRelease, got %v", err)
	}
}
