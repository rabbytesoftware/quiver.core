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

func TestResolveFunc_CacheHit(t *testing.T) {
	cached := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "pkg"}}
	rawContent := []byte(`name: pkg`)

	v := &mocks.Vault{
		GetArrowFile: vault.ManifestFile{Content: rawContent, Filename: "arrow.yaml"},
		GetArrowErr:  nil,
	}
	m := &mocks.Manifold{
		ParseArrowResult: cached,
	}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Name != cached.Name {
		t.Fatalf("expected cached manifest name %q, got %v", cached.Name, got)
	}
	if v.PutArrowCalls != 0 {
		t.Fatal("vault PutArrow should not have been called")
	}
}

func TestResolveFunc_Stale_RefreshSucceeds(t *testing.T) {
	staleContent := []byte(`name: stale`)
	fresh := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "fresh"}}

	v := &mocks.Vault{
		GetArrowFile: vault.ManifestFile{Content: staleContent, Filename: "arrow.yaml"},
		GetArrowErr:  vault.ErrStale,
	}
	m := &mocks.Manifold{
		ResolveArrowResult:   fresh,
		ResolveArrowRaw:      []byte(`name: fresh`),
		ResolveArrowFilename: "ARROW.md",
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

func TestResolveFunc_Stale_RefreshFails(t *testing.T) {
	staleContent := []byte(`name: stale`)
	stale := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "stale"}}
	manifoldErr := errors.New("network timeout")

	v := &mocks.Vault{
		GetArrowFile: vault.ManifestFile{Content: staleContent, Filename: "arrow.yaml"},
		GetArrowErr:  vault.ErrStale,
	}
	m := &mocks.Manifold{
		ResolveArrowErr:  manifoldErr,
		ParseArrowResult: stale,
	}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err != nil {
		t.Fatalf("expected no error on degraded stale, got: %v", err)
	}
	if got == nil || got.Name != stale.Name {
		t.Fatalf("expected stale manifest name %q, got %v", stale.Name, got)
	}
	if v.PutArrowCalls != 0 {
		t.Fatal("vault PutArrow should not have been called on degraded stale")
	}
}

func TestResolveFunc_Miss(t *testing.T) {
	fresh := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "fresh"}}

	v := &mocks.Vault{
		GetArrowErr: vault.ErrNotCached,
	}
	m := &mocks.Manifold{
		ResolveArrowResult:   fresh,
		ResolveArrowRaw:      []byte(`name: fresh`),
		ResolveArrowFilename: "ARROW.md",
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

func TestResolveFunc_Miss_ResolveFails(t *testing.T) {
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

func TestResolveFunc_OtherVaultError(t *testing.T) {
	vaultErr := errors.New("permission denied")

	v := &mocks.Vault{
		GetArrowErr: vaultErr,
	}
	m := &mocks.Manifold{}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err == nil {
		t.Fatal("expected error when vault returns unexpected error")
	}
	if got != nil {
		t.Fatal("expected nil manifest on error")
	}
}

func TestResolveFunc_CacheHit_ParseFails(t *testing.T) {
	rawContent := []byte(`name: pkg`)
	parseErr := errors.New("bad yaml")

	v := &mocks.Vault{
		GetArrowFile: vault.ManifestFile{Content: rawContent, Filename: "arrow.yaml"},
		GetArrowErr:  nil,
	}
	m := &mocks.Manifold{
		ParseArrowErr: parseErr,
	}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err == nil {
		t.Fatal("expected error when parse fails on cache hit")
	}
	if got != nil {
		t.Fatal("expected nil manifest on error")
	}
}

func TestResolveFunc_Stale_RefreshFails_ParseStaleFails(t *testing.T) {
	staleContent := []byte(`name: stale`)
	manifoldErr := errors.New("network timeout")
	parseErr := errors.New("bad stale yaml")

	v := &mocks.Vault{
		GetArrowFile: vault.ManifestFile{Content: staleContent, Filename: "arrow.yaml"},
		GetArrowErr:  vault.ErrStale,
	}
	m := &mocks.Manifold{
		ResolveArrowErr: manifoldErr,
		ParseArrowErr:   parseErr,
	}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err == nil {
		t.Fatal("expected error when both refresh and stale parse fail")
	}
	if got != nil {
		t.Fatal("expected nil manifest on error")
	}
}

func TestResolveFunc_Stale_RefreshSucceeds_PutFails(t *testing.T) {
	staleContent := []byte(`name: stale`)
	fresh := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "fresh"}}
	putErr := errors.New("disk full")

	v := &mocks.Vault{
		GetArrowFile: vault.ManifestFile{Content: staleContent, Filename: "arrow.yaml"},
		GetArrowErr:  vault.ErrStale,
		PutArrowErr:  putErr,
	}
	m := &mocks.Manifold{
		ResolveArrowResult:   fresh,
		ResolveArrowRaw:      []byte(`name: fresh`),
		ResolveArrowFilename: "ARROW.md",
	}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err == nil {
		t.Fatal("expected error when PutArrow fails after stale refresh")
	}
	if got != nil {
		t.Fatal("expected nil manifest on error")
	}
}

func TestResolveFunc_Miss_PutFails(t *testing.T) {
	fresh := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "fresh"}}
	putErr := errors.New("disk full")

	v := &mocks.Vault{
		GetArrowErr: vault.ErrNotCached,
		PutArrowErr: putErr,
	}
	m := &mocks.Manifold{
		ResolveArrowResult:   fresh,
		ResolveArrowRaw:      []byte(`name: fresh`),
		ResolveArrowFilename: "ARROW.md",
	}

	resolve := manifest.New(v, m)
	got, err := resolve(context.Background(), testNS)

	if err == nil {
		t.Fatal("expected error when PutArrow fails on cache miss")
	}
	if got != nil {
		t.Fatal("expected nil manifest on error")
	}
}
