package resolver

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// mockFetch captures the last call arguments and returns configured bytes/err.
type mockFetch struct {
	data       []byte
	err        error
	gotURL     string
	gotPath    string
	gotTimeout time.Duration
}

func (m *mockFetch) call(
	_ context.Context,
	cloneURL string,
	filePath string,
	timeout time.Duration,
) ([]byte, error) {
	m.gotURL = cloneURL
	m.gotPath = filePath
	m.gotTimeout = timeout

	return m.data, m.err
}

func newTestResolver(
	timeout time.Duration,
	mock *mockFetch,
) *Resolver {
	r := New(timeout)
	r.fetch = mock.call

	return r
}

// ─── ResolveArrow ─────────────────────────────────────────────────────────────

func TestResolveArrow_ThreeSegments(t *testing.T) {
	mock := &mockFetch{data: []byte("yaml")}
	r := newTestResolver(5*time.Second, mock)

	data, err := r.ResolveArrow(context.Background(), "github.com/user/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "yaml" {
		t.Errorf("data = %q, want yaml", data)
	}
	if mock.gotURL != "https://github.com/user/repo" {
		t.Errorf("cloneURL = %q, want https://github.com/user/repo", mock.gotURL)
	}
	if mock.gotPath != "arrow.yaml" {
		t.Errorf("filePath = %q, want arrow.yaml", mock.gotPath)
	}
}

func TestResolveArrow_FourSegments(t *testing.T) {
	mock := &mockFetch{data: []byte("yaml")}
	r := newTestResolver(5*time.Second, mock)

	_, err := r.ResolveArrow(context.Background(), "github.com/user/repo/myarrow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.gotURL != "https://github.com/user/repo" {
		t.Errorf("cloneURL = %q, want https://github.com/user/repo", mock.gotURL)
	}
	if mock.gotPath != "myarrow.yaml" {
		t.Errorf("filePath = %q, want myarrow.yaml", mock.gotPath)
	}
}

func TestResolveArrow_Platforms(t *testing.T) {
	platforms := []struct {
		namespace domain.Namespace
		wantURL   string
	}{
		{"github.com/u/r", "https://github.com/u/r"},
		{"gitlab.com/u/r", "https://gitlab.com/u/r"},
		{"bitbucket.org/u/r", "https://bitbucket.org/u/r"},
	}

	for _, tc := range platforms {
		mock := &mockFetch{data: []byte("yaml")}
		r := newTestResolver(5*time.Second, mock)

		_, err := r.ResolveArrow(context.Background(), tc.namespace)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.namespace, err)
			continue
		}
		if mock.gotURL != tc.wantURL {
			t.Errorf("%s: cloneURL = %q, want %q", tc.namespace, mock.gotURL, tc.wantURL)
		}
	}
}

func TestResolveArrow_InvalidNamespace(t *testing.T) {
	r := New(5 * time.Second)

	_, err := r.ResolveArrow(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

func TestResolveArrow_TwoSegmentNamespace(t *testing.T) {
	r := New(5 * time.Second)

	_, err := r.ResolveArrow(context.Background(), "github.com/user")
	if err == nil {
		t.Fatal("expected error for two-segment namespace")
	}
}

func TestResolveArrow_UnsupportedPlatform(t *testing.T) {
	r := New(5 * time.Second)

	_, err := r.ResolveArrow(context.Background(), "codeberg.org/user/repo")
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform, got %v", err)
	}
}

func TestResolveArrow_FetchError(t *testing.T) {
	fetchErr := fmt.Errorf("%w: something", ErrFetchFailed)
	mock := &mockFetch{err: fetchErr}
	r := newTestResolver(5*time.Second, mock)

	_, err := r.ResolveArrow(context.Background(), "github.com/user/repo")
	if !errors.Is(err, ErrFetchFailed) {
		t.Errorf("expected ErrFetchFailed, got %v", err)
	}
}

func TestResolveArrow_TimeoutPassthrough(t *testing.T) {
	mock := &mockFetch{data: []byte("yaml")}
	r := newTestResolver(42*time.Second, mock)

	_, _ = r.ResolveArrow(context.Background(), "github.com/user/repo")
	if mock.gotTimeout != 42*time.Second {
		t.Errorf("timeout = %v, want 42s", mock.gotTimeout)
	}
}

// ─── ResolveQuiver ────────────────────────────────────────────────────────────

func TestResolveQuiver_AlwaysQuiverYAML(t *testing.T) {
	namespaces := []domain.Namespace{
		"github.com/user/repo",
		"github.com/user/repo/something",
		"gitlab.com/org/project",
	}

	for _, ns := range namespaces {
		mock := &mockFetch{data: []byte("yaml")}
		r := newTestResolver(5*time.Second, mock)

		_, err := r.ResolveQuiver(context.Background(), ns)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", ns, err)
			continue
		}
		if mock.gotPath != "quiver.yaml" {
			t.Errorf("%s: filePath = %q, want quiver.yaml", ns, mock.gotPath)
		}
	}
}

func TestResolveQuiver_UnsupportedPlatform(t *testing.T) {
	r := New(5 * time.Second)

	_, err := r.ResolveQuiver(context.Background(), "codeberg.org/user/repo")
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform, got %v", err)
	}
}

func TestResolveQuiver_FetchError(t *testing.T) {
	fetchErr := fmt.Errorf("%w: something", ErrNotFound)
	mock := &mockFetch{err: fetchErr}
	r := newTestResolver(5*time.Second, mock)

	_, err := r.ResolveQuiver(context.Background(), "github.com/user/repo")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ─── Default timeout ─────────────────────────────────────────────────────────

func TestNew_DefaultTimeout(t *testing.T) {
	mock := &mockFetch{data: []byte("yaml")}
	r := New(0) // zero → should use 30s default
	r.fetch = mock.call

	_, _ = r.ResolveArrow(context.Background(), "github.com/user/repo")
	if mock.gotTimeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", mock.gotTimeout)
	}
}
