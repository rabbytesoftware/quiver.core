package collection_test

import (
	"testing"

	collection "github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/collection"
)

func TestNewRegistry(t *testing.T) {
	r := collection.NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() returned nil")
	}
}

func TestRegistry_Get_V0(t *testing.T) {
	r := collection.NewRegistry()
	m, err := r.Get("v0")
	if err != nil {
		t.Fatalf("Get(v0) error = %v", err)
	}
	if m == nil {
		t.Fatal("Get(v0) returned nil module")
	}
	if m.Version() != "v0" {
		t.Errorf("Version() = %q, want v0", m.Version())
	}
}

func TestRegistry_Get_UnsupportedVersion(t *testing.T) {
	r := collection.NewRegistry()
	_, err := r.Get("v999")
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestRegistry_Get_EmptyVersion(t *testing.T) {
	r := collection.NewRegistry()
	_, err := r.Get("")
	if err == nil {
		t.Fatal("expected error for empty version")
	}
}

func TestRegistry_MultipleInstances(t *testing.T) {
	r1 := collection.NewRegistry()
	r2 := collection.NewRegistry()

	m1, err1 := r1.Get("v0")
	m2, err2 := r2.Get("v0")

	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}

	if m1.Version() != m2.Version() {
		t.Error("modules have different versions")
	}
}
