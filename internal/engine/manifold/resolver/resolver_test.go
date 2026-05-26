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
	r := New(0)
	if r == nil {
		t.Fatal("New(0) returned nil")
	}
}

func TestNew_WithCustomTimeout(t *testing.T) {
	r := New(10 * time.Second)
	if r == nil {
		t.Fatal("New(10s) returned nil")
	}
}

func TestResolveArrow_InvalidNamespace_Empty(t *testing.T) {
	r := New(5 * time.Second)
	_, _, err := r.ResolveArrow(context.Background(), domain.Namespace(""))
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

func TestResolveArrow_InvalidNamespace_TwoSegments(t *testing.T) {
	r := New(5 * time.Second)
	_, _, err := r.ResolveArrow(context.Background(), domain.Namespace("github.com/user"))
	if err == nil {
		t.Fatal("expected error for two-segment namespace")
	}
}

func TestResolveCollection_InvalidNamespace_Empty(t *testing.T) {
	r := New(5 * time.Second)
	_, err := r.ResolveCollection(context.Background(), domain.Namespace(""))
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

func TestResolveCollection_InvalidNamespace_TwoSegments(t *testing.T) {
	r := New(5 * time.Second)
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
	r := New(5 * time.Second)
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

func TestResolveArrow_AUID_MarkdownFirst(t *testing.T) {
	fetcher := &stubFetcher{
		canResolve:  true,
		data:        []byte("auid-md-manifest"),
		err:         nil,
		acceptPaths: map[string]bool{"cs2.md": true},
	}
	r := &resolver{
		timeout:  5 * time.Second,
		fetchers: []resolvers.Fetcher{fetcher},
	}
	data, filename, err := r.ResolveArrow(context.Background(), domain.Namespace("github.com/char2cs/gaming.quiver/cs2"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "auid-md-manifest" {
		t.Errorf("data = %q, want auid-md-manifest", data)
	}
	if filename != "cs2.md" {
		t.Errorf("filename = %q, want cs2.md", filename)
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
