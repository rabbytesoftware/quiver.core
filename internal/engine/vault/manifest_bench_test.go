package vault

import (
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

func largeArrowManifest() domain.ArrowManifest {
	deps := make([]domain.Namespace, 50)
	for i := range deps {
		deps[i] = domain.Namespace("example.com/user/repo")
	}
	vars := make([]domain.Variable, 20)
	for i := range vars {
		vars[i] = domain.Variable{
			Name:        "VAR",
			Description: "a variable",
			Default:     "default",
		}
	}
	return domain.ArrowManifest{
		Name:         "large-arrow",
		Description:  "a large manifest for benchmarking purposes",
		Version:      "1.0.0",
		License:      "MIT",
		URL:          "https://example.com",
		Maintainers:  []string{"user1", "user2", "user3"},
		Tags:         []string{"tag1", "tag2", "tag3", "tag4", "tag5"},
		Dependencies: deps,
		Variables:    vars,
	}
}

func BenchmarkGetManifest_SmallArrow(b *testing.B) {
	s := &store{basePath: b.TempDir(), ttl: time.Hour, osVersion: "darwin/arm64"}
	ns := domain.Namespace("example.com/user/repo")
	manifest := domain.ArrowManifest{Name: "small-arrow", Version: "1.0.0"}

	_, err := putManifest[domain.ArrowManifest](s, ns, "arrow.json", &manifest)
	if err != nil {
		b.Fatalf("setup failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = getManifest[domain.ArrowManifest](s, ns, "arrow.json")
	}
}

func BenchmarkGetManifest_LargeArrow(b *testing.B) {
	s := &store{basePath: b.TempDir(), ttl: time.Hour, osVersion: "darwin/arm64"}
	ns := domain.Namespace("example.com/user/repo")
	manifest := largeArrowManifest()

	_, err := putManifest[domain.ArrowManifest](s, ns, "arrow.json", &manifest)
	if err != nil {
		b.Fatalf("setup failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = getManifest[domain.ArrowManifest](s, ns, "arrow.json")
	}
}

func BenchmarkPutManifest_SmallArrow(b *testing.B) {
	s := &store{basePath: b.TempDir(), ttl: time.Hour, osVersion: "darwin/arm64"}
	ns := domain.Namespace("example.com/user/repo")
	manifest := domain.ArrowManifest{Name: "small-arrow", Version: "1.0.0"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = putManifest[domain.ArrowManifest](s, ns, "arrow.json", &manifest)
	}
}

func BenchmarkPutManifest_LargeArrow(b *testing.B) {
	s := &store{basePath: b.TempDir(), ttl: time.Hour, osVersion: "darwin/arm64"}
	ns := domain.Namespace("example.com/user/repo")
	manifest := largeArrowManifest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = putManifest[domain.ArrowManifest](s, ns, "arrow.json", &manifest)
	}
}
