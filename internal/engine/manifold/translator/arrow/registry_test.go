package arrow_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/arrow"
)

func TestNew_ReturnsRegistry(t *testing.T) {
	r := arrow.New()
	if r == nil {
		t.Fatal("New() returned nil")
	}
	m, err := r.Get("v0")
	if err != nil {
		t.Fatalf("New().Get(v0) error = %v", err)
	}
	if m == nil {
		t.Fatal("New().Get(v0) returned nil module")
	}
}

func TestNewRegistry(t *testing.T) {
	r := arrow.NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() returned nil")
	}
}

func TestRegistry_Get_V0(t *testing.T) {
	r := arrow.NewRegistry()
	m, err := r.Get("v0")
	if err != nil {
		t.Fatalf("Get(v0) error = %v", err)
	}
	if m == nil {
		t.Fatal("Get(v0) returned nil module")
	}
}

func TestRegistry_Get_UnsupportedVersion(t *testing.T) {
	r := arrow.NewRegistry()
	_, err := r.Get("v999")
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestRegistry_Get_EmptyVersion(t *testing.T) {
	r := arrow.NewRegistry()
	_, err := r.Get("")
	if err == nil {
		t.Fatal("expected error for empty version")
	}
}

func TestRegistry_MultipleInstances(t *testing.T) {
	r1 := arrow.NewRegistry()
	r2 := arrow.NewRegistry()

	m1, err1 := r1.Get("v0")
	m2, err2 := r2.Get("v0")

	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}

	if m1 == nil || m2 == nil {
		t.Error("modules are nil")
	}
}
