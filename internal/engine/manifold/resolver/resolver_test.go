package resolver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
)

func TestNew_WithZeroTimeout(t *testing.T) {
	r := New(0, nil)
	if r == nil {
		t.Fatal("New(0) returned nil")
	}
}

func TestNew_WithCustomTimeout(t *testing.T) {
	r := New(10*time.Second, nil)
	if r == nil {
		t.Fatal("New(10s) returned nil")
	}
}

func TestResolveArrow_InvalidNamespace_Empty(t *testing.T) {
	r := New(5*time.Second, nil)
	_, _, err := r.ResolveArrow(context.Background(), domain.Namespace(""))
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

func TestResolveArrow_InvalidNamespace_TwoSegments(t *testing.T) {
	r := New(5*time.Second, nil)
	_, _, err := r.ResolveArrow(context.Background(), domain.Namespace("github.com/user"))
	if err == nil {
		t.Fatal("expected error for two-segment namespace")
	}
}

func TestResolveCollection_InvalidNamespace_Empty(t *testing.T) {
	r := New(5*time.Second, nil)
	_, err := r.ResolveCollection(context.Background(), domain.Namespace(""))
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

func TestResolveCollection_InvalidNamespace_TwoSegments(t *testing.T) {
	r := New(5*time.Second, nil)
	_, err := r.ResolveCollection(context.Background(), domain.Namespace("github.com/user"))
	if err == nil {
		t.Fatal("expected error for two-segment namespace")
	}
}

// ─── Stub fetcher for testing ──────────────────────────────────────────────────

type stubFetcher struct {
	canResolve  bool
	data        []byte
	err         error
	acceptPaths map[string]bool
	called      bool
}

func (s *stubFetcher) CanResolve(_ domain.Namespace) bool {
	return s.canResolve
}

func (s *stubFetcher) Fetch(
	_ context.Context,
	_ domain.Namespace,
	filePaths []string,
	_ time.Duration,
) ([]byte, string, error) {
	s.called = true
	if s.acceptPaths == nil {
		if s.err != nil {
			return nil, "", s.err
		}
		return s.data, filePaths[0], nil
	}
	for _, filePath := range filePaths {
		if s.acceptPaths[filePath] {
			return s.data, filePath, s.err
		}
	}
	return nil, "", resolvers.ErrNotFound
}

// ─── Orchestrator tests with stub fetchers ────────────────────────────────────

func TestFetchManifest_FirstFetcherSucceeds(t *testing.T) {
	fetcher := &stubFetcher{
		canResolve: true,
		data:       []byte("manifest"),
		err:        nil,
	}
	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher},
	}

	data, _, err := r.fetchManifest(context.Background(), domain.Namespace("github.com/user/repo"), []string{"arrow.yaml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "manifest" {
		t.Errorf("data = %q, want manifest", data)
	}
}

func TestFetchManifest_FirstFetcherFails_SecondSucceeds(t *testing.T) {
	fetcher1 := &stubFetcher{
		canResolve: true,
		data:       nil,
		err:        resolvers.ErrFetchFailed,
	}
	fetcher2 := &stubFetcher{
		canResolve: true,
		data:       []byte("manifest"),
		err:        nil,
	}

	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher1, fetcher2},
	}

	data, _, err := r.fetchManifest(context.Background(), domain.Namespace("github.com/user/repo"), []string{"arrow.yaml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "manifest" {
		t.Errorf("data = %q, want manifest", data)
	}
}

func TestFetchManifest_CanResolveFalse_Skipped(t *testing.T) {
	fetcher1 := &stubFetcher{
		canResolve: false,
		data:       nil,
		err:        nil,
	}
	fetcher2 := &stubFetcher{
		canResolve: true,
		data:       []byte("manifest"),
		err:        nil,
	}

	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher1, fetcher2},
	}

	data, _, err := r.fetchManifest(context.Background(), domain.Namespace("github.com/user/repo"), []string{"arrow.yaml"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "manifest" {
		t.Errorf("data = %q, want manifest", data)
	}
}

func TestFetchManifest_AllFail_ReturnsFirstNonNotFoundError(t *testing.T) {
	fetcher1 := &stubFetcher{
		canResolve: true,
		data:       nil,
		err:        resolvers.ErrFetchFailed,
	}
	fetcher2 := &stubFetcher{
		canResolve: true,
		data:       nil,
		err:        resolvers.ErrNotFound,
	}

	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher1, fetcher2},
	}

	_, _, err := r.fetchManifest(context.Background(), domain.Namespace("github.com/user/repo"), []string{"arrow.yaml"})
	if !errors.Is(err, resolvers.ErrFetchFailed) {
		t.Errorf("error = %v, want resolvers.ErrFetchFailed", err)
	}
	if errors.Is(err, ErrManifestNotFound) {
		t.Errorf("error = %v, must not be ErrManifestNotFound", err)
	}
}

func TestFetchManifest_NoFetchersCanResolve_ReturnsError(t *testing.T) {
	fetcher1 := &stubFetcher{
		canResolve: false,
		data:       nil,
		err:        nil,
	}
	fetcher2 := &stubFetcher{
		canResolve: false,
		data:       nil,
		err:        nil,
	}

	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher1, fetcher2},
	}

	_, _, err := r.fetchManifest(context.Background(), domain.Namespace("github.com/user/repo"), []string{"arrow.yaml"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, resolvers.ErrFetchFailed) {
		t.Errorf("error = %v, want resolvers.ErrFetchFailed", err)
	}
}

func TestResolveArrow_Success(t *testing.T) {
	fetcher := &stubFetcher{
		canResolve: true,
		data:       []byte("ok"),
		err:        nil,
	}
	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher},
	}
	data, _, err := r.ResolveArrow(context.Background(), domain.Namespace("github.com/user/repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "ok" {
		t.Errorf("data = %q, want ok", data)
	}
}

func TestResolveCollection_Success(t *testing.T) {
	fetcher := &stubFetcher{
		canResolve: true,
		data:       []byte("ok"),
		err:        nil,
	}
	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher},
	}
	data, err := r.ResolveCollection(context.Background(), domain.Namespace("github.com/user/repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "ok" {
		t.Errorf("data = %q, want ok", data)
	}
}

func TestResolveArrow_WithAUID(t *testing.T) {
	fetcher := &stubFetcher{
		canResolve: true,
		data:       []byte("manifest"),
		err:        nil,
	}
	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher},
	}
	data, _, err := r.ResolveArrow(context.Background(), domain.Namespace("github.com/user/repo@abc123"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "manifest" {
		t.Errorf("data = %q, want manifest", data)
	}
}

func TestResolveArrow_InvalidNamespace_BadFormat(t *testing.T) {
	r := New(5*time.Second, nil)
	_, _, err := r.ResolveArrow(context.Background(), domain.Namespace("invalid"))
	if err == nil {
		t.Fatal("expected error for invalid namespace format")
	}
}

func TestResolveArrow_WithAUID_FetchesSpecificFile(t *testing.T) {
	fetcher := &stubFetcher{
		canResolve: true,
		data:       []byte("auid-manifest"),
		err:        nil,
	}
	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher},
	}
	data, _, err := r.ResolveArrow(context.Background(), domain.Namespace("github.com/user/repo@abc123"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "auid-manifest" {
		t.Errorf("data = %q, want auid-manifest", data)
	}
}

func TestResolveArrow_ReturnsFilename_MarkdownFirst(t *testing.T) {
	fetcher := &stubFetcher{
		canResolve:  true,
		data:        []byte("md-manifest"),
		err:         nil,
		acceptPaths: map[string]bool{"ARROW.md": true},
	}
	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher},
	}
	data, filename, err := r.ResolveArrow(context.Background(), domain.Namespace("github.com/user/repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "md-manifest" {
		t.Errorf("data = %q, want md-manifest", data)
	}
	if filename != "ARROW.md" {
		t.Errorf("filename = %q, want ARROW.md", filename)
	}
}

func TestResolveArrow_FallsBackToYAML_WhenMarkdownNotFound(t *testing.T) {
	fetcher := &stubFetcher{
		canResolve:  true,
		data:        []byte("yaml-manifest"),
		err:         nil,
		acceptPaths: map[string]bool{"arrow.yaml": true},
	}
	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher},
	}
	data, filename, err := r.ResolveArrow(context.Background(), domain.Namespace("github.com/user/repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "yaml-manifest" {
		t.Errorf("data = %q, want yaml-manifest", data)
	}
	if filename != "arrow.yaml" {
		t.Errorf("filename = %q, want arrow.yaml", filename)
	}
}

func TestResolveArrowAt_Success(t *testing.T) {
	fetcher := &stubFetcher{
		canResolve:  true,
		data:        []byte("nested-manifest"),
		err:         nil,
		acceptPaths: map[string]bool{"tools/appimage-runtime.yaml": true},
	}
	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher},
	}
	data, filename, err := r.ResolveArrowAt(
		context.Background(),
		domain.Namespace("github.com/rabbytesoftware/quiver.essentials/appimage-runtime"),
		"tools/appimage-runtime",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "nested-manifest" {
		t.Errorf("data = %q, want nested-manifest", data)
	}
	if filename != "tools/appimage-runtime.yaml" {
		t.Errorf("filename = %q, want tools/appimage-runtime.yaml", filename)
	}
}

func TestResolveArrowAt_MarkdownFirst(t *testing.T) {
	fetcher := &stubFetcher{
		canResolve:  true,
		data:        []byte("nested-md-manifest"),
		err:         nil,
		acceptPaths: map[string]bool{"tools/appimage-runtime.md": true},
	}
	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher},
	}
	_, filename, err := r.ResolveArrowAt(
		context.Background(),
		domain.Namespace("github.com/rabbytesoftware/quiver.essentials/appimage-runtime"),
		"tools/appimage-runtime",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filename != "tools/appimage-runtime.md" {
		t.Errorf("filename = %q, want tools/appimage-runtime.md", filename)
	}
}

func TestResolveArrowAt_EmptyPath_ReturnsError(t *testing.T) {
	r := New(5*time.Second, nil)
	_, _, err := r.ResolveArrowAt(
		context.Background(),
		domain.Namespace("github.com/rabbytesoftware/quiver.essentials/appimage-runtime"),
		"",
	)
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestResolveArrowAt_InvalidNamespace_ReturnsError(t *testing.T) {
	r := New(5*time.Second, nil)
	_, _, err := r.ResolveArrowAt(context.Background(), domain.Namespace("invalid"), "tools/x")
	if err == nil {
		t.Fatal("expected error for invalid namespace format")
	}
}

type rawHost struct {
	base string
}

func (h rawHost) RawFileURL(
	ns domain.Namespace,
	ref string,
	file string,
) (string, error) {
	segments := strings.Split(string(ns.BareNamespace()), "/")
	return h.base + "/" + segments[1] + "/" + segments[2] + "/" + ref + "/" + file, nil
}

func (h rawHost) BlobFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", nil
}

func (h rawHost) OwnerAvatarURL(
	_ domain.Namespace,
) string {
	return ""
}

func (h rawHost) RepoPageURL(
	_ domain.Namespace,
) string {
	return ""
}

func (h rawHost) RepoMetadata(
	_ context.Context,
	_ domain.Namespace,
) (domain.RepoMetadata, error) {
	return domain.RepoMetadata{}, nil
}

func (h rawHost) ReleaseAssets(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) ([]domain.ReleaseAsset, error) {
	return nil, nil
}

func (h rawHost) DefaultBranches() []string {
	return []string{"main"}
}

func (h rawHost) LatestRelease(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "", errors.New("unused")
}

func statusServer(
	t *testing.T,
	status int,
) hosts.Lookup {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return func(_ domain.Namespace) (hosts.Host, bool) {
		return rawHost{base: server.URL}, true
	}
}

func TestFetchManifest_ClassifiesAbsence(t *testing.T) {
	gitNoneFound := fmt.Errorf("%w: none of [ARROW.md arrow.yaml] found", resolvers.ErrNotFound)
	gitCloneFailed := fmt.Errorf("%w: clone https://example: dial tcp: i/o timeout", resolvers.ErrFetchFailed)

	testCases := []struct {
		name              string
		namespace         domain.Namespace
		httpStatus        int
		gitErr            error
		wantManifestMiss  bool
		wantIs            error
		wantGitErr        error
		wantGitFetcherRan bool
	}{
		{name: "http 404 and git none found", namespace: "github.com/user/repo", httpStatus: http.StatusNotFound, gitErr: gitNoneFound, wantManifestMiss: true, wantIs: resolvers.ErrNotFound, wantGitFetcherRan: true},
		{name: "http 500 and git not found", namespace: "github.com/user/repo@v1.0.0", httpStatus: http.StatusInternalServerError, gitErr: gitNoneFound, wantIs: resolvers.ErrFetchFailed, wantGitFetcherRan: true},
		{name: "http 503 and git not found", namespace: "github.com/user/repo@v1.0.0", httpStatus: http.StatusServiceUnavailable, gitErr: gitNoneFound, wantIs: resolvers.ErrFetchFailed, wantGitFetcherRan: true},
		{name: "http rate limited and git not found", namespace: "github.com/user/repo@v1.0.0", httpStatus: http.StatusTooManyRequests, gitErr: gitNoneFound, wantIs: resolvers.ErrFetchFailed, wantGitFetcherRan: true},
		{name: "refless http 404 and git transport failure", namespace: "github.com/user/repo", httpStatus: http.StatusNotFound, gitErr: gitCloneFailed, wantIs: resolvers.ErrFetchFailed, wantGitErr: gitCloneFailed, wantGitFetcherRan: true},
		{name: "pinned ref http 404 skips git transport failure", namespace: "github.com/user/repo@v1.0.0", httpStatus: http.StatusNotFound, gitErr: gitCloneFailed, wantManifestMiss: true, wantIs: resolvers.ErrNotFound, wantGitFetcherRan: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gitFetcher := &stubFetcher{canResolve: true, err: tc.gitErr}
			r := &resolver{
				timeout: 5 * time.Second,
				fetchers: []resolvers.Fetcher{
					resolvers.NewHTTP(statusServer(t, tc.httpStatus)),
					gitFetcher,
				},
			}

			_, _, err := r.ResolveArrow(context.Background(), tc.namespace)

			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantIs)
			assert.Equal(t, tc.wantManifestMiss, errors.Is(err, ErrManifestNotFound))
			assert.Equal(t, tc.wantGitFetcherRan, gitFetcher.called,
				"a pinned ref absent at the http fetcher must short-circuit before trying git")
			if tc.wantGitErr != nil {
				assert.ErrorIs(t, err, tc.wantGitErr)
			}
		})
	}
}

func TestFetchManifest_OnlyGitRanAndFoundNothing_IsManifestNotFound(t *testing.T) {
	r := &resolver{
		timeout: 5 * time.Second,
		fetchers: []resolvers.Fetcher{
			&stubFetcher{canResolve: false},
			&stubFetcher{canResolve: true, err: resolvers.ErrNotFound},
		},
	}

	_, _, err := r.ResolveArrow(context.Background(), domain.Namespace("example.org/user/repo"))

	assert.ErrorIs(t, err, ErrManifestNotFound)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestFetchManifest_CollectionAbsence_IsManifestNotFound(t *testing.T) {
	r := &resolver{
		timeout: 5 * time.Second,
		fetchers: []resolvers.Fetcher{
			&stubFetcher{canResolve: true, err: resolvers.ErrNotFound},
		},
	}

	_, err := r.ResolveCollection(context.Background(), domain.Namespace("github.com/user/repo"))

	assert.ErrorIs(t, err, ErrManifestNotFound)
}
