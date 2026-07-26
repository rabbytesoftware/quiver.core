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

	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestHTTPFetcher_CanResolve_KnownDomain(t *testing.T) {
	platforms := metadata.Platforms{
		"github.com": {
			RawURL:          "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
			DefaultBranches: []string{"main"},
		},
	}
	fetcher := NewHTTP(platforms)

	ok := fetcher.CanResolve(domain.Namespace("github.com/user/repo"))
	if !ok {
		t.Error("CanResolve(github.com/user/repo) = false, want true")
	}
}

func TestHTTPFetcher_CanResolve_UnknownDomain(t *testing.T) {
	platforms := metadata.Platforms{
		"github.com": {
			RawURL:          "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
			DefaultBranches: []string{"main"},
		},
	}
	fetcher := NewHTTP(platforms)

	ok := fetcher.CanResolve(domain.Namespace("custom.example.com/user/repo"))
	if ok {
		t.Error("CanResolve(custom.example.com/user/repo) = true, want false")
	}
}

func TestHTTPFetcher_Fetch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("schema: arrow@v0\nname: test\n"))
	}))
	defer server.Close()

	platforms := metadata.Platforms{
		"example.com": {
			RawURL:          server.URL + "/{file}",
			DefaultBranches: []string{"main"},
		},
	}
	fetcher := NewHTTP(platforms)

	data, err := fetcher.Fetch(context.Background(), domain.Namespace("example.com/user/repo"), "arrow.yaml", 5*time.Second)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if string(data) != "schema: arrow@v0\nname: test\n" {
		t.Errorf("Fetch() data = %q, want schema: arrow@v0\\nname: test\\n", string(data))
	}
}

func TestHTTPFetcher_Fetch_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	platforms := metadata.Platforms{
		"example.com": {
			RawURL:          server.URL + "/{file}",
			DefaultBranches: []string{"main"},
		},
	}
	fetcher := NewHTTP(platforms)

	_, err := fetcher.Fetch(context.Background(), domain.Namespace("example.com/user/repo"), "arrow.yaml", 5*time.Second)
	if err == nil {
		t.Fatal("Fetch() expected error, got nil")
	}
	if !isErrNotFound(err) {
		t.Errorf("Fetch() error = %v, want ErrNotFound", err)
	}
}

func TestHTTPFetcher_Fetch_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	platforms := metadata.Platforms{
		"example.com": {
			RawURL:          server.URL + "/{file}",
			DefaultBranches: []string{"main"},
		},
	}
	fetcher := NewHTTP(platforms)

	_, err := fetcher.Fetch(context.Background(), domain.Namespace("example.com/user/repo"), "arrow.yaml", 5*time.Second)
	if err == nil {
		t.Fatal("Fetch() expected error, got nil")
	}
}

func TestHTTPFetcher_Fetch_Timeout(t *testing.T) {
	// Use a handler that blocks with context-aware timeout
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(10 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			http.Error(w, "context cancelled", http.StatusRequestTimeout)
		}
	}))
	defer server.Close()

	platforms := metadata.Platforms{
		"example.com": {
			RawURL:          server.URL + "/{file}",
			DefaultBranches: []string{"main"},
		},
	}
	fetcher := NewHTTP(platforms)

	_, err := fetcher.Fetch(context.Background(), domain.Namespace("example.com/user/repo"), "arrow.yaml", 1*time.Millisecond)
	if err == nil {
		t.Fatal("Fetch() expected timeout error, got nil")
	}
}

func TestHTTPFetcher_Fetch_UsesRefAsBranch(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("schema: arrow@v0\n"))
	}))
	defer server.Close()

	platforms := metadata.Platforms{
		"example.com": {
			RawURL:          server.URL + "/{user}/{repo}/{branch}/{file}",
			DefaultBranches: []string{"main"},
		},
	}
	fetcher := NewHTTP(platforms)

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

func TestBuildRawURL_GitHub(t *testing.T) {
	template := "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}"
	url := buildRawURL(template, "myuser", "myrepo", "main", "arrow.yaml")

	expected := "https://raw.githubusercontent.com/myuser/myrepo/main/arrow.yaml"
	if url != expected {
		t.Errorf("buildRawURL() = %q, want %q", url, expected)
	}
}

func TestBuildRawURL_GitLab(t *testing.T) {
	template := "https://gitlab.com/{user}/{repo}/-/raw/{branch}/{file}"
	url := buildRawURL(template, "myuser", "myrepo", "main", "arrow.yaml")

	expected := "https://gitlab.com/myuser/myrepo/-/raw/main/arrow.yaml"
	if url != expected {
		t.Errorf("buildRawURL() = %q, want %q", url, expected)
	}
}

func TestBuildRawURL_Bitbucket(t *testing.T) {
	template := "https://bitbucket.org/{user}/{repo}/raw/{branch}/{file}"
	url := buildRawURL(template, "myuser", "myrepo", "main", "arrow.yaml")

	expected := "https://bitbucket.org/myuser/myrepo/raw/main/arrow.yaml"
	if url != expected {
		t.Errorf("buildRawURL() = %q, want %q", url, expected)
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

func branchPlatforms(
	serverURL string,
	branches []string,
) metadata.Platforms {
	return metadata.Platforms{
		"example.com": {
			RawURL:          serverURL + "/{user}/{repo}/{branch}/{file}",
			DefaultBranches: branches,
		},
	}
}

func TestHTTPFetcher_MainOnlyRepo_ResolvesWithoutExtraRequest(t *testing.T) {
	server, paths := branchServer(t, map[string]bool{"main": true})
	fetcher := NewHTTP(branchPlatforms(server.URL, []string{"main", "master"}))

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
	fetcher := NewHTTP(branchPlatforms(server.URL, []string{"main", "master"}))

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
	fetcher := NewHTTP(branchPlatforms(server.URL, []string{"main", "master"}))

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
	fetcher := NewHTTP(branchPlatforms(server.URL, []string{"main", "master"}))

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
	fetcher := NewHTTP(branchPlatforms(server.URL, []string{"main", "master"}))

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

	fetcher := NewHTTP(branchPlatforms(server.URL, []string{"main", "master"}))

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
	fetcher := NewHTTP(branchPlatforms(server.URL, nil))

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

func isErrNotFound(err error) bool {
	return isError(err, ErrNotFound)
}

func isError(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		// Try to unwrap
		type unwrapper interface {
			Unwrap() error
		}
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}
