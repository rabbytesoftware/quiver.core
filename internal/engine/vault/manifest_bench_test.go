package vault

import (
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/engine/vault/mocks"
)

var testManifestFile = ManifestFile{Content: []byte("# test arrow"), Filename: "ARROW.md"}

func newBenchStore(b *testing.B) *store {
	b.Helper()
	return &store{
		vaultPath:      b.TempDir(),
		namespacesPath: b.TempDir(),
		ttl:            time.Hour,
		clock:          time.Now,
		locks:          make(map[string]*sync.Mutex),
	}
}

// Benchmarks for getArrow
func BenchmarkGetArrow(b *testing.B) {
	s := newBenchStore(b)
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
	s := newBenchStore(b)
	ns := mocks.Namespace()
	manifest := mocks.Quiver()

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
	s := newBenchStore(b)
	ns := mocks.Namespace()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = putArrow(s, ns, testManifestFile)
	}
}

// Benchmarks for putQuiver
func BenchmarkPutQuiver(b *testing.B) {
	s := newBenchStore(b)
	ns := mocks.Namespace()
	manifest := mocks.Quiver()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = putQuiver(s, ns, manifest)
	}
}

// Benchmarks for deleteArrow
func BenchmarkDeleteArrow(b *testing.B) {
	s := newBenchStore(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := mocks.Namespace()
		_ = putArrow(s, ns, testManifestFile)
		_ = deleteArrow(s, ns)
	}
}

// Benchmarks for deleteQuiver
func BenchmarkDeleteQuiver(b *testing.B) {
	s := newBenchStore(b)
	manifest := mocks.Quiver()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := mocks.Namespace()
		_, _ = putQuiver(s, ns, manifest)
		_ = deleteQuiver(s, ns)
	}
}

// Benchmark atomic write pattern (common operation)
func BenchmarkAtomicWrite(b *testing.B) {
	s := newBenchStore(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ns := mocks.Namespace()
		_ = putArrow(s, ns, testManifestFile)
	}
}

// Benchmark concurrent access pattern
func BenchmarkConcurrentGetArrow(b *testing.B) {
	s := newBenchStore(b)
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
	s := newBenchStore(b)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ns := mocks.Namespace()
			_ = putArrow(s, ns, testManifestFile)
		}
	})
}
