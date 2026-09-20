package resolver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
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
}

func (s *stubFetcher) CanResolve(_ domain.Namespace) bool {
	return s.canResolve
}

func (s *stubFetcher) Fetch(
	_ context.Context,
	_ domain.Namespace,
	filePath string,
	_ time.Duration,
) ([]byte, error) {
	if s.acceptPaths != nil {
		if !s.acceptPaths[filePath] {
			return nil, resolvers.ErrNotFound
		}
	}
	return s.data, s.err
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

func TestFetchManifest_AllFail_ReturnsLastError(t *testing.T) {
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
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, resolvers.ErrNotFound) {
		t.Errorf("error = %v, want resolvers.ErrNotFound", err)
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
