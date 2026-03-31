package resolver_test

import (
	"context"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/resolver"
)

func TestNew_WithZeroTimeout(t *testing.T) {
	r := resolver.New(0)
	if r == nil {
		t.Fatal("New(0) returned nil")
	}
}

func TestNew_WithCustomTimeout(t *testing.T) {
	r := resolver.New(10 * time.Second)
	if r == nil {
		t.Fatal("New(10s) returned nil")
	}
}

func TestResolveArrow_InvalidNamespace_Empty(t *testing.T) {
	r := resolver.New(5 * time.Second)
	_, err := r.ResolveArrow(context.Background(), domain.Namespace(""))
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

func TestResolveArrow_InvalidNamespace_TwoSegments(t *testing.T) {
	r := resolver.New(5 * time.Second)
	_, err := r.ResolveArrow(context.Background(), domain.Namespace("github.com/user"))
	if err == nil {
		t.Fatal("expected error for two-segment namespace")
	}
}

func TestResolveQuiver_InvalidNamespace_Empty(t *testing.T) {
	r := resolver.New(5 * time.Second)
	_, err := r.ResolveQuiver(context.Background(), domain.Namespace(""))
	if err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

func TestResolveQuiver_InvalidNamespace_TwoSegments(t *testing.T) {
	r := resolver.New(5 * time.Second)
	_, err := r.ResolveQuiver(context.Background(), domain.Namespace("github.com/user"))
	if err == nil {
		t.Fatal("expected error for two-segment namespace")
	}
}
