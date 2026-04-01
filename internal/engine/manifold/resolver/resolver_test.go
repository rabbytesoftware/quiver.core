package resolver

import (
	"context"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
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
	_, err := r.ResolveArrow(context.Background(), domain.Namespace(""))
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

func TestResolveArrow_InvalidNamespace_TwoSegments(t *testing.T) {
	r := New(5 * time.Second)
	_, err := r.ResolveArrow(context.Background(), domain.Namespace("github.com/user"))
	if err == nil {
		t.Fatal("expected error for two-segment namespace")
	}
}

func TestResolveQuiver_InvalidNamespace_Empty(t *testing.T) {
	r := New(5 * time.Second)
	_, err := r.ResolveQuiver(context.Background(), domain.Namespace(""))
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

func TestResolveQuiver_InvalidNamespace_TwoSegments(t *testing.T) {
	r := New(5 * time.Second)
	_, err := r.ResolveQuiver(context.Background(), domain.Namespace("github.com/user"))
	if err == nil {
		t.Fatal("expected error for two-segment namespace")
	}
}

// ─── Happy-path tests with stub fetcher ────────────────────────────────────

func stubFetcher(_ context.Context, _ string, _ string, _ time.Duration) ([]byte, error) {
	return []byte("ok"), nil
}

func TestResolve_ValidNamespace_BuildsCloneURL(t *testing.T) {
	cloneURL, parts, err := resolve(domain.Namespace("github.com/user/repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cloneURL != "https://github.com/user/repo" {
		t.Errorf("cloneURL = %q, want https://github.com/user/repo", cloneURL)
	}
	if len(parts) != 3 {
		t.Errorf("parts count = %d, want 3", len(parts))
	}
}

func TestResolveArrowParts_ThreePart_DefaultsToArrowYaml(t *testing.T) {
	_, filePath, err := resolveArrowParts(domain.Namespace("github.com/user/repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filePath != "arrow.yaml" {
		t.Errorf("filePath = %q, want arrow.yaml", filePath)
	}
}

func TestResolveArrowParts_FourPart_UsesNamedYaml(t *testing.T) {
	_, filePath, err := resolveArrowParts(domain.Namespace("github.com/user/repo/my-arrow"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filePath != "my-arrow.yaml" {
		t.Errorf("filePath = %q, want my-arrow.yaml", filePath)
	}
}

func TestResolveArrow_Success(t *testing.T) {
	r := &resolver{
		timeout: 5 * time.Second,
		fetch:   stubFetcher,
	}
	data, err := r.ResolveArrow(context.Background(), domain.Namespace("github.com/user/repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "ok" {
		t.Errorf("data = %q, want ok", data)
	}
}

func TestResolveQuiver_Success(t *testing.T) {
	r := &resolver{
		timeout: 5 * time.Second,
		fetch:   stubFetcher,
	}
	data, err := r.ResolveQuiver(context.Background(), domain.Namespace("github.com/user/repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "ok" {
		t.Errorf("data = %q, want ok", data)
	}
}
