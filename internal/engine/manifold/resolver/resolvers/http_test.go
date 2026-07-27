package resolvers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

// stubHost is a git host as this fetcher sees one: a raw-file URL and a list of
// refs to try. Building the URL is the host's business, which is why the shape
// under test here is only which URLs get asked for, and in what order.
type stubHost struct {
	rawURL   string
	branches []string
	urlErr   error
}

func (s stubHost) RawFileURL(
	ns domain.Namespace,
	ref string,
	file string,
) (string, error) {
	if s.urlErr != nil {
		return "", s.urlErr
	}

	segments := strings.Split(string(ns.BareNamespace()), domain.NamespaceSeparator)
	return strings.NewReplacer(
		"{user}", segments[1],
		"{repo}", segments[2],
		"{branch}", ref,
		"{file}", file,
	).Replace(s.rawURL), nil
}

func (s stubHost) DefaultBranches() []string {
	return s.branches
}

func (s stubHost) LatestRelease(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "", errors.New("the fetcher never asks this")
}

// hostFor answers for one domain only, which is what makes "no host serves
// this namespace" a case the fetcher can be shown.
func hostFor(
	host string,
	h hosts.Host,
) hosts.Lookup {
	return func(ns domain.Namespace) (hosts.Host, bool) {
		if ns.Domain() != host {
			return nil, false
		}
		return h, true
	}
}

func serverHost(
	serverURL string,
	branches []string,
) hosts.Lookup {
	return hostFor("example.com", stubHost{
		rawURL:   serverURL + "/{user}/{repo}/{branch}/{file}",
		branches: branches,
	})
}

// ─── CanResolve ──────────────────────────────────────────────────────────────

func TestHTTPFetcher_CanResolve_KnownDomain(t *testing.T) {
	fetcher := NewHTTP(hostFor("github.com", stubHost{rawURL: "https://raw/{file}"}))

	if !fetcher.CanResolve(domain.Namespace("github.com/user/repo")) {
		t.Error("CanResolve(github.com/user/repo) = false, want true")
	}
}

func TestHTTPFetcher_CanResolve_UnknownDomain(t *testing.T) {
	fetcher := NewHTTP(hostFor("github.com", stubHost{rawURL: "https://raw/{file}"}))

	if fetcher.CanResolve(domain.Namespace("custom.example.com/user/repo")) {
		t.Error("CanResolve(custom.example.com/user/repo) = true, want false")
	}
}

// A fetcher wired with no lookup at all knows no hosts, which is a miss and not
// a panic: the git fetcher resolves that namespace instead.
func TestHTTPFetcher_NilLookup_ResolvesNothing(t *testing.T) {
	fetcher := NewHTTP(nil)

	if fetcher.CanResolve(domain.Namespace("github.com/user/repo")) {
		t.Error("CanResolve with no lookup = true, want false")
	}

	_, err := fetcher.Fetch(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
		"arrow.yaml",
		time.Second,
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Fetch() error = %v, want ErrNotFound", err)
	}
}

// ─── Fetch ───────────────────────────────────────────────────────────────────

func TestHTTPFetcher_Fetch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("schema: arrow@v0\nname: test\n"))
	}))
	defer server.Close()

	fetcher := NewHTTP(serverHost(server.URL, []string{"main"}))

	data, err := fetcher.Fetch(context.Background(), domain.Namespace("example.com/user/repo"), "arrow.yaml", 5*time.Second)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if string(data) != "schema: arrow@v0\nname: test\n" {
		t.Errorf("Fetch() data = %q, want schema: arrow@v0\\nname: test\\n", string(data))
	}
}

func TestHTTPFetcher_Fetch_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := NewHTTP(serverHost(server.URL, []string{"main"}))

	_, err := fetcher.Fetch(context.Background(), domain.Namespace("example.com/user/repo"), "arrow.yaml", 5*time.Second)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Fetch() error = %v, want ErrNotFound", err)
	}
}

func TestHTTPFetcher_Fetch_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fetcher := NewHTTP(serverHost(server.URL, []string{"main"}))

	_, err := fetcher.Fetch(context.Background(), domain.Namespace("example.com/user/repo"), "arrow.yaml", 5*time.Second)
	if err == nil {
		t.Fatal("Fetch() expected error, got nil")
	}
}

func TestHTTPFetcher_Fetch_Timeout(t *testing.T) {
	// A handler that blocks until its own request context is cancelled, so the
	// fetcher's deadline is what ends the exchange.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		http.Error(w, "context cancelled", http.StatusRequestTimeout)
	}))
	defer server.Close()

	fetcher := NewHTTP(serverHost(server.URL, []string{"main"}))

	_, err := fetcher.Fetch(context.Background(), domain.Namespace("example.com/user/repo"), "arrow.yaml", time.Millisecond)
	if err == nil {
		t.Fatal("Fetch() expected timeout error, got nil")
	}
}

func TestHTTPFetcher_Fetch_UsesRefAsBranch(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("schema: arrow@v0\n"))
	}))
	defer server.Close()

	fetcher := NewHTTP(serverHost(server.URL, []string{"main"}))

	_, err := fetcher.Fetch(context.Background(), domain.Namespace("example.com/user/repo@v1.2.3"), "arrow.yaml", 5*time.Second)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	// Path must be /user/repo/v1.2.3/arrow.yaml — ref in branch position, repo clean
	want := "/user/repo/v1.2.3/arrow.yaml"
	if capturedPath != want {
		t.Errorf("Fetch() URL path = %q, want %q", capturedPath, want)
	}
}

// A host that cannot name a URL cannot serve the file, and saying so is not the
// same as proving the file absent: the resolver falls through to cloning.
func TestHTTPFetcher_Fetch_HostCannotNameAURL(t *testing.T) {
	fetcher := NewHTTP(hostFor("example.com", stubHost{
		urlErr:   errors.New("host serves no raw files"),
		branches: []string{"main"},
	}))

	_, err := fetcher.Fetch(
		context.Background(),
		domain.Namespace("example.com/user/repo"),
		"arrow.yaml",
		5*time.Second,
	)
	if !errors.Is(err, ErrFetchFailed) {
		t.Fatalf("Fetch() error = %v, want ErrFetchFailed", err)
	}
}

// TestFetchHTTP_InvalidURL covers the http.NewRequestWithContext error branch
// by passing a URL with a control character that makes request construction fail.
func TestFetchHTTP_InvalidURL(t *testing.T) {
	_, err := fetchHTTP(context.Background(), "http://\x00invalid", 5*time.Second)
	if err == nil {
		t.Fatal("fetchHTTP() expected error for invalid URL, got nil")
	}
	if !errors.Is(err, ErrFetchFailed) {
		t.Errorf("fetchHTTP() error = %v, want ErrFetchFailed", err)
	}
}

// ─── default branch list ─────────────────────────────────────────────────────

// branchServer serves arrow.yaml only on the branches listed in available and
// records every path it was asked for, in order.
func branchServer(
	t *testing.T,
	available map[string]bool,
) (*httptest.Server, *[]string) {
	t.Helper()

	var mu sync.Mutex
	paths := make([]string, 0, 4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()

		segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		branch := segments[len(segments)-2]
		if !available[branch] {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("branch: " + branch + "\n"))
	}))
	t.Cleanup(server.Close)

	return server, &paths
}

func TestHTTPFetcher_MainOnlyRepo_ResolvesWithoutExtraRequest(t *testing.T) {
	server, paths := branchServer(t, map[string]bool{"main": true})
	fetcher := NewHTTP(serverHost(server.URL, []string{"main", "master"}))

	data, err := fetcher.Fetch(
		context.Background(),
		domain.Namespace("example.com/user/repo"),
		"arrow.yaml",
		5*time.Second,
	)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if string(data) != "branch: main\n" {
		t.Errorf("Fetch() data = %q, want %q", string(data), "branch: main\n")
	}

	want := []string{"/user/repo/main/arrow.yaml"}
	if !reflect.DeepEqual(*paths, want) {
		t.Errorf("requested paths = %v, want %v", *paths, want)
	}
}

func TestHTTPFetcher_MasterOnlyRepo_ResolvesAfterOne404(t *testing.T) {
	server, paths := branchServer(t, map[string]bool{"master": true})
	fetcher := NewHTTP(serverHost(server.URL, []string{"main", "master"}))

	data, err := fetcher.Fetch(
		context.Background(),
		domain.Namespace("example.com/user/repo"),
		"arrow.yaml",
		5*time.Second,
	)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if string(data) != "branch: master\n" {
		t.Errorf("Fetch() data = %q, want %q", string(data), "branch: master\n")
	}

	want := []string{"/user/repo/main/arrow.yaml", "/user/repo/master/arrow.yaml"}
	if !reflect.DeepEqual(*paths, want) {
		t.Errorf("requested paths = %v, want %v", *paths, want)
	}
}

func TestHTTPFetcher_NeitherBranch_ReturnsNotFound(t *testing.T) {
	server, paths := branchServer(t, map[string]bool{})
	fetcher := NewHTTP(serverHost(server.URL, []string{"main", "master"}))

	_, err := fetcher.Fetch(
		context.Background(),
		domain.Namespace("example.com/user/repo"),
		"arrow.yaml",
		5*time.Second,
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Fetch() error = %v, want ErrNotFound", err)
	}

	want := []string{"/user/repo/main/arrow.yaml", "/user/repo/master/arrow.yaml"}
	if !reflect.DeepEqual(*paths, want) {
		t.Errorf("requested paths = %v, want %v", *paths, want)
	}
}

func TestHTTPFetcher_ExplicitRef_SkipsTheListEntirely(t *testing.T) {
	server, paths := branchServer(t, map[string]bool{"v1.2.3": true})
	fetcher := NewHTTP(serverHost(server.URL, []string{"main", "master"}))

	data, err := fetcher.Fetch(
		context.Background(),
		domain.Namespace("example.com/user/repo@v1.2.3"),
		"arrow.yaml",
		5*time.Second,
	)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if string(data) != "branch: v1.2.3\n" {
		t.Errorf("Fetch() data = %q, want %q", string(data), "branch: v1.2.3\n")
	}

	want := []string{"/user/repo/v1.2.3/arrow.yaml"}
	if !reflect.DeepEqual(*paths, want) {
		t.Errorf("requested paths = %v, want %v", *paths, want)
	}
}

func TestHTTPFetcher_ExplicitRefMissing_DoesNotFallBackToTheList(t *testing.T) {
	server, paths := branchServer(t, map[string]bool{"main": true})
	fetcher := NewHTTP(serverHost(server.URL, []string{"main", "master"}))

	_, err := fetcher.Fetch(
		context.Background(),
		domain.Namespace("example.com/user/repo@v9.9.9"),
		"arrow.yaml",
		5*time.Second,
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Fetch() error = %v, want ErrNotFound", err)
	}

	want := []string{"/user/repo/v9.9.9/arrow.yaml"}
	if !reflect.DeepEqual(*paths, want) {
		t.Errorf("requested paths = %v, want %v", *paths, want)
	}
}

// A non-404 is not evidence that the branch is missing, so the list must not
// advance past it.
func TestHTTPFetcher_ServerError_AbortsWithoutTryingTheNextBranch(t *testing.T) {
	var mu sync.Mutex
	paths := make([]string, 0, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fetcher := NewHTTP(serverHost(server.URL, []string{"main", "master"}))

	_, err := fetcher.Fetch(
		context.Background(),
		domain.Namespace("example.com/user/repo"),
		"arrow.yaml",
		5*time.Second,
	)
	if !errors.Is(err, ErrFetchFailed) {
		t.Fatalf("Fetch() error = %v, want ErrFetchFailed", err)
	}

	want := []string{"/user/repo/main/arrow.yaml"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("requested paths = %v, want %v", paths, want)
	}
}

func TestHTTPFetcher_EmptyBranchList_ReturnsNotFoundWithoutRequesting(t *testing.T) {
	server, paths := branchServer(t, map[string]bool{"main": true})
	fetcher := NewHTTP(serverHost(server.URL, nil))

	_, err := fetcher.Fetch(
		context.Background(),
		domain.Namespace("example.com/user/repo"),
		"arrow.yaml",
		5*time.Second,
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Fetch() error = %v, want ErrNotFound", err)
	}
	if len(*paths) != 0 {
		t.Errorf("requested paths = %v, want none", *paths)
	}
}
