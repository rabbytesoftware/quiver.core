package providers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
)

const (
	glabNamespace       = "gitlab.com/gitlab-org/cli"
	glabTag             = "v1.119.0"
	glabReleaseURI      = "/api/v4/projects/gitlab-org%2Fcli/releases/v1.119.0"
	glabPackagesURI     = "/api/v4/projects/gitlab-org%2Fcli/packages?package_name=glab&package_type=generic&package_version=1.119.0"
	glabFilesURI        = "/api/v4/projects/gitlab-org%2Fcli/packages/70206788/package_files?page=1&per_page=100"
	glabChecksumsURI    = "/gitlab-org/cli/-/releases/v1.119.0/downloads/checksums.txt"
	glabChecksumsPkgURI = "/api/v4/projects/gitlab-org%2Fcli/packages/generic/glab/1%2E119%2E0/checksums%2Etxt"
)

type cannedResponse struct {
	status   int
	body     string
	headers  map[string]string
	location string
	hangup   bool
}

type fakeGitLab struct {
	server *httptest.Server
	client *http.Client
	mu     sync.Mutex
	routes map[string]cannedResponse
	hits   []string
}

func newFakeGitLab(
	t *testing.T,
) *fakeGitLab {
	t.Helper()
	f := &fakeGitLab{routes: map[string]cannedResponse{}}
	f.server = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	f.client = f.server.Client()
	f.client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return f
}

func (f *fakeGitLab) serve(
	w http.ResponseWriter,
	r *http.Request,
) {
	f.mu.Lock()
	f.hits = append(f.hits, r.RequestURI)
	resp, ok := f.routes[r.RequestURI]
	f.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if resp.hangup {
		hangUp(w)
		return
	}
	for key, value := range resp.headers {
		w.Header().Set(key, value)
	}
	location := resp.location
	if strings.HasPrefix(location, "/") {
		location = f.server.URL + location
	}
	if location != "" {
		w.Header().Set("Location", location)
	}
	w.WriteHeader(resp.status)
	_, _ = w.Write([]byte(resp.body))
}

func hangUp(
	w http.ResponseWriter,
) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	_ = conn.Close()
}

func (f *fakeGitLab) do(
	ctx context.Context,
	req fns.Request,
) (fns.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return fns.Response{}, err
	}
	for key, values := range req.Headers {
		httpReq.Header[key] = values
	}

	resp, err := f.client.Do(httpReq)
	if err != nil {
		return fns.Response{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fns.Response{}, err
	}
	return fns.Response{Status: resp.StatusCode, Headers: resp.Header, Body: body}, nil
}

func (f *fakeGitLab) route(
	uri string,
	resp cannedResponse,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[uri] = resp
}

func (f *fakeGitLab) ok(
	uri string,
	body string,
) {
	f.route(uri, cannedResponse{status: http.StatusOK, body: body})
}

func (f *fakeGitLab) count(
	uri string,
) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, hit := range f.hits {
		if hit == uri {
			n++
		}
	}
	return n
}

func (f *fakeGitLab) fixture(
	t *testing.T,
	name string,
) string {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return strings.ReplaceAll(string(body), "https://gitlab.com", f.server.URL)
}

func (f *fakeGitLab) serveGlab(
	t *testing.T,
) {
	t.Helper()
	f.ok(glabReleaseURI, f.fixture(t, "gitlab_release_glab.json"))
	f.ok(glabPackagesURI, f.fixture(t, "gitlab_packages_glab.json"))
	f.ok(glabFilesURI, f.fixture(t, "gitlab_package_files_glab.json"))
	f.route(glabChecksumsURI, cannedResponse{status: http.StatusFound, location: glabChecksumsPkgURI})
	f.ok(glabChecksumsPkgURI, f.fixture(t, "gitlab_checksums_glab.txt"))
}

func (f *fakeGitLab) onServer(
	link gitlabLink,
) gitlabLink {
	if strings.HasPrefix(link.URL, "/") {
		link.URL = f.server.URL + link.URL
	}
	if strings.HasPrefix(link.DirectAssetURL, "/") {
		link.DirectAssetURL = f.server.URL + link.DirectAssetURL
	}
	return link
}

func (f *fakeGitLab) config() Config {
	base := f.server.URL
	return Config{
		Host:           "gitlab.com",
		RawURL:         base + "/{user}/{repo}/-/raw/{branch}/{file}",
		BlobURL:        base + "/{user}/{repo}/-/blob/{branch}/{file}",
		ReleaseAPIURL:  base + "/api/v4/projects/{user}%2F{repo}/releases/{tag}",
		RepoPageURL:    base + "/{user}/{repo}",
		PackagesAPIURL: base + "/api/v4/projects/{project}/packages",
		Timeout:        5 * time.Second,
		Do:             f.do,
	}
}

func (f *fakeGitLab) provider() Provider {
	return NewGitLab(f.config())
}
