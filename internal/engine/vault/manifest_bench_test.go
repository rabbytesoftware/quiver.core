package vault

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault/mocks"
)

// Benchmarks for getArrow
func BenchmarkGetArrow(b *testing.B) {
	s := newTestStore(&testing.T{})
	ns := mocks.Namespace()
	manifest := mocks.ArrowManifest()

	_, err := putArrow(s, ns, manifest, nil)
	if err != nil {
		b.Fatalf("Failed to setup: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = getArrow(s, ns)
	}
}

func BenchmarkGetArrowWithIndirectDeps(b *testing.B) {
	s := newTestStore(&testing.T{})
	ns := mocks.Namespace()
	manifest := mocks.ArrowManifest()
	indirectDeps := []string{"github.com/foo/bar", "github.com/baz/qux"}

	// Convert string slice to Namespace slice
	deps := make([]interface{}, len(indirectDeps))
	for i, d := range indirectDeps {
		deps[i] = d
	}

	_, err := putArrow(s, ns, manifest, nil)
	if err != nil {
		b.Fatalf("Failed to setup: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = getArrow(s, ns)
	}
}

// Benchmarks for getQuiver
func BenchmarkGetQuiver(b *testing.B) {
	s := newTestStore(&testing.T{})
	ns := mocks.Namespace()
	manifest := mocks.QuiverManifest()

	_, err := putQuiver(s, ns, manifest)
	if err != nil {
		b.Fatalf("Failed to setup: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = getQuiver(s, ns)
	}
}

// Benchmarks for putArrow
func BenchmarkPutArrow(b *testing.B) {
	s := newTestStore(&testing.T{})
	ns := mocks.Namespace()
	manifest := mocks.ArrowManifest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = putArrow(s, ns, manifest, nil)
	}
}

func BenchmarkPutArrowWithIndirectDeps(b *testing.B) {
	s := newTestStore(&testing.T{})
	manifest := mocks.ArrowManifest()
	indirectDeps := []domain.Namespace{
		domain.Namespace("github.com/foo/bar"),
		domain.Namespace("github.com/baz/qux"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := mocks.Namespace()
		_, _ = putArrow(s, ns, manifest, indirectDeps)
	}
}

// Benchmarks for putQuiver
func BenchmarkPutQuiver(b *testing.B) {
	s := newTestStore(&testing.T{})
	ns := mocks.Namespace()
	manifest := mocks.QuiverManifest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = putQuiver(s, ns, manifest)
	}
}

// Benchmarks for deleteArrow
func BenchmarkDeleteArrow(b *testing.B) {
	s := newTestStore(&testing.T{})
	manifest := mocks.ArrowManifest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := mocks.Namespace()
		putArrow(s, ns, manifest, nil)
		deleteArrow(s, ns)
	}
}

// Benchmarks for deleteQuiver
func BenchmarkDeleteQuiver(b *testing.B) {
	s := newTestStore(&testing.T{})
	manifest := mocks.QuiverManifest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := mocks.Namespace()
		putQuiver(s, ns, manifest)
		deleteQuiver(s, ns)
	}
}

// Benchmark atomic write pattern (common operation)
func BenchmarkAtomicWrite(b *testing.B) {
	s := newTestStore(&testing.T{})
	manifest := mocks.ArrowManifest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := mocks.Namespace()
		putArrow(s, ns, manifest, nil)
	}
}

// Benchmark concurrent access pattern
func BenchmarkConcurrentGetArrow(b *testing.B) {
	s := newTestStore(&testing.T{})
	ns := mocks.Namespace()
	manifest := mocks.ArrowManifest()

	_, err := putArrow(s, ns, manifest, nil)
	if err != nil {
		b.Fatalf("Failed to setup: %v", err)
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = getArrow(s, ns)
		}
	})
}

// Benchmark concurrent put pattern
func BenchmarkConcurrentPutArrow(b *testing.B) {
	s := newTestStore(&testing.T{})
	manifest := mocks.ArrowManifest()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ns := mocks.Namespace()
			_, _ = putArrow(s, ns, manifest, nil)
		}
	})
}
