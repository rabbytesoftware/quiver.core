package manifest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/manifest"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
)

const testNS domain.Namespace = "github.com/test/pkg@v1.0.0"

func TestNew_CacheHit(t *testing.T) {
	cached := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "pkg"}}
	v := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: cached},
		GetArrowErr:   nil,
	}
	m := &mocks.Manifold{}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != cached {
		t.Fatalf("expected cached manifest, got different pointer")
	}
	if m.ResolveArrowManifest != nil || m.ResolveArrowErr != nil {
		t.Fatal("manifold should not have been called")
	}
	if v.PutArrowCalls != 0 {
		t.Fatal("vault PutArrow should not have been called")
	}
}

func TestNew_StaleCache_ManifoldSuccess(t *testing.T) {
	stale := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "stale"}}
	fresh := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "fresh"}}

	v := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: stale},
		GetArrowErr:   vault.ErrStale,
		PutArrowPath:  "/some/path",
	}
	m := &mocks.Manifold{
		ResolveArrowManifest: fresh,
	}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fresh {
		t.Fatalf("expected fresh manifest, got stale")
	}
	if v.PutArrowCalls != 1 {
		t.Fatalf("expected PutArrow called once, got %d", v.PutArrowCalls)
	}
}

func TestNew_StaleCache_ManifoldFails_DegradesToStale(t *testing.T) {
	stale := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "stale"}}
	manifoldErr := errors.New("network timeout")

	v := &mocks.Vault{
		GetArrowEntry: &vault.VaultEntry{Manifest: stale},
		GetArrowErr:   vault.ErrStale,
	}
	m := &mocks.Manifold{
		ResolveArrowErr: manifoldErr,
	}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err != nil {
		t.Fatalf("expected no error on degraded stale, got: %v", err)
	}
	if got != stale {
		t.Fatalf("expected stale manifest on manifold failure, got different pointer")
	}
	if v.PutArrowCalls != 0 {
		t.Fatal("vault PutArrow should not have been called on degraded stale")
	}
}

func TestNew_NotCached_ManifoldSuccess(t *testing.T) {
	fresh := &domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "fresh"}}

	v := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/some/path",
	}
	m := &mocks.Manifold{
		ResolveArrowManifest: fresh,
	}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fresh {
		t.Fatalf("expected fresh manifest, got different pointer")
	}
	if v.PutArrowCalls != 1 {
		t.Fatalf("expected PutArrow called once, got %d", v.PutArrowCalls)
	}
}

func TestNew_NotCached_ManifoldFails(t *testing.T) {
	manifoldErr := errors.New("remote unavailable")

	v := &mocks.Vault{
		GetArrowErr: vault.ErrNotCached,
	}
	m := &mocks.Manifold{
		ResolveArrowErr: manifoldErr,
	}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err == nil {
		t.Fatal("expected error when manifold fails on cache miss")
	}
	if got != nil {
		t.Fatal("expected nil manifest on error")
	}
	if v.PutArrowCalls != 0 {
		t.Fatal("vault PutArrow should not have been called")
	}
}
