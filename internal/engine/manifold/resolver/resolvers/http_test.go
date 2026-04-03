package resolvers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

func TestHTTPFetcher_CanResolve_KnownDomain(t *testing.T) {
	platforms := metadata.Platforms{
		"github.com": {
			RawURL:        "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
			DefaultBranch: "main",
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
			RawURL:        "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
			DefaultBranch: "main",
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
			RawURL:        server.URL + "/{file}",
			DefaultBranch: "main",
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
			RawURL:        server.URL + "/{file}",
			DefaultBranch: "main",
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
			RawURL:        server.URL + "/{file}",
			DefaultBranch: "main",
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
			RawURL:        server.URL + "/{file}",
			DefaultBranch: "main",
		},
	}
	fetcher := NewHTTP(platforms)

	_, err := fetcher.Fetch(context.Background(), domain.Namespace("example.com/user/repo"), "arrow.yaml", 1*time.Millisecond)
	if err == nil {
		t.Fatal("Fetch() expected timeout error, got nil")
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

func isErrNotFound(err error) bool {
	return isError(err, ErrNotFound)
}

func isError(err error, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		// Try to unwrap
		type unwrapper interface {
			Unwrap() error
		}
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
		} else {
			break
		}
	}
	return false
}
