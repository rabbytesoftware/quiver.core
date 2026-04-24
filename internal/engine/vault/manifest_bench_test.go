package vault

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/engine/vault/mocks"
)

var testManifestFile = ManifestFile{Content: []byte("# test arrow"), Filename: "ARROW.md"}

// Benchmarks for getArrow
func BenchmarkGetArrow(b *testing.B) {
	s := newTestStore(&testing.T{})
	ns := mocks.Namespace()

	if err := putArrow(s, ns, testManifestFile); err != nil {
		b.Fatalf("Failed to setup: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = getArrow(s, ns)
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = putArrow(s, ns, testManifestFile)
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := mocks.Namespace()
		_ = putArrow(s, ns, testManifestFile)
		_ = deleteArrow(s, ns)
	}
}

// Benchmarks for deleteQuiver
func BenchmarkDeleteQuiver(b *testing.B) {
	s := newTestStore(&testing.T{})
	manifest := mocks.QuiverManifest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := mocks.Namespace()
		_, _ = putQuiver(s, ns, manifest)
		_ = deleteQuiver(s, ns)
	}
}

// Benchmark atomic write pattern (common operation)
func BenchmarkAtomicWrite(b *testing.B) {
	s := newTestStore(&testing.T{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := mocks.Namespace()
		_ = putArrow(s, ns, testManifestFile)
	}
}

// Benchmark concurrent access pattern
func BenchmarkConcurrentGetArrow(b *testing.B) {
	s := newTestStore(&testing.T{})
	ns := mocks.Namespace()

	if err := putArrow(s, ns, testManifestFile); err != nil {
		b.Fatalf("Failed to setup: %v", err)
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = getArrow(s, ns)
		}
	})
}

// Benchmark concurrent put pattern
func BenchmarkConcurrentPutArrow(b *testing.B) {
	s := newTestStore(&testing.T{})

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ns := mocks.Namespace()
			_ = putArrow(s, ns, testManifestFile)
		}
	})
}
