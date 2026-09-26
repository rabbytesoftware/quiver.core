package store_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

// resolveViaManifest wraps a store.Store.ResolveManifest call with a freshly-built store.
func resolveViaManifest(
	t *testing.T,
	v vault.Vault,
	m *mocks.Manifold,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	t.Helper()
	r := newTestReaderWithVaultManifold(t, v, m)
	return r.ResolveManifest(context.Background(), ns)
}

func TestResolver_NilVaultNilManifold_Error(t *testing.T) {
	r := newTestReader(t)
	_, err := r.ResolveManifest(context.Background(), domain.Namespace("github.com/user/pkg@v1"))
	require.Error(t, err)
}

func TestResolver_VaultHit_ParseSuccess(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Cached"}}
	v := &mocks.Vault{
		GetArrowFile: vault.ManifestFile{Content: []byte("raw")},
	}
	m := &mocks.Manifold{ParseArrowResult: arrow}

	got, err := resolveViaManifest(t, v, m, ns)
	require.NoError(t, err)
	assert.Equal(t, "Cached", got.Name)
}

func TestResolver_VaultStale_ManifoldFetchSuccess(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Fresh"}}
	v := &mocks.Vault{
		GetArrowErr:  vault.ErrStale,
		GetArrowFile: vault.ManifestFile{Content: []byte("stale")},
	}
	m := &mocks.Manifold{
		ResolveArrowResult:   arrow,
		ResolveArrowRaw:      []byte("fresh"),
		ResolveArrowFilename: "ARROW.md",
	}

	got, err := resolveViaManifest(t, v, m, ns)
	require.NoError(t, err)
	assert.Equal(t, "Fresh", got.Name)
}

func TestResolver_VaultStale_ManifoldFetchFails_FallbackToStale(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "StaleResult"}}
	v := &mocks.Vault{
		GetArrowErr:  vault.ErrStale,
		GetArrowFile: vault.ManifestFile{Content: []byte("stale")},
	}
	m := &mocks.Manifold{
		ResolveArrowErr:  errors.New("network error"),
		ParseArrowResult: arrow,
	}

	got, err := resolveViaManifest(t, v, m, ns)
	require.NoError(t, err)
	assert.Equal(t, "StaleResult", got.Name)
}

func TestResolver_VaultNotCached_ManifoldFetchSuccess(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Fetched"}}
	v := &mocks.Vault{
		GetArrowErr: vault.ErrNotCached,
	}
	m := &mocks.Manifold{
		ResolveArrowResult:   arrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}

	got, err := resolveViaManifest(t, v, m, ns)
	require.NoError(t, err)
	assert.Equal(t, "Fetched", got.Name)
}

func TestResolver_VaultError_Other(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{
		GetArrowErr: errors.New("internal vault error"),
	}
	m := &mocks.Manifold{}

	_, err := resolveViaManifest(t, v, m, ns)
	require.Error(t, err)
}

func TestResolver_NoVault_ManifoldOnly(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "ManifoldOnly"}}
	m := &mocks.Manifold{
		ResolveArrowResult:   arrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}

	r := newTestReaderWithVaultManifold(t, nil, m)
	got, err := r.ResolveManifest(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, "ManifoldOnly", got.Name)
}

func TestResolver_NilVault_ManifoldFetchError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	m := &mocks.Manifold{
		ResolveArrowErr: errors.New("manifold error"),
	}

	r := newTestReaderWithVaultManifold(t, nil, m)
	_, err := r.ResolveManifest(context.Background(), ns)
	require.Error(t, err)
}

func TestResolver_VaultStale_NilManifold_Error(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{
		GetArrowErr:  vault.ErrStale,
		GetArrowFile: vault.ManifestFile{Content: []byte("stale")},
	}
	// nil manifold interface + ErrStale → resolver returns error
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	r, err := store.New(db, v, nil) // nil manifold.Manifold interface
	require.NoError(t, err)
	_, err = r.ResolveManifest(context.Background(), ns)
	require.Error(t, err)
}

func TestResolver_VaultStale_PutArrowError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Fresh"}}
	v := &mocks.Vault{
		GetArrowErr:  vault.ErrStale,
		GetArrowFile: vault.ManifestFile{Content: []byte("stale")},
		PutArrowErr:  errors.New("storage full"),
	}
	m := &mocks.Manifold{
		ResolveArrowResult:   arrow,
		ResolveArrowRaw:      []byte("fresh"),
		ResolveArrowFilename: "ARROW.md",
	}

	r := newTestReaderWithVaultManifold(t, v, m)
	_, err := r.ResolveManifest(context.Background(), ns)
	require.Error(t, err)
}

func TestResolver_VaultNotCached_NilManifold_Error(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{
		GetArrowErr: vault.ErrNotCached,
	}
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	r, err := store.New(db, v, nil) // nil manifold.Manifold
	require.NoError(t, err)
	_, err = r.ResolveManifest(context.Background(), ns)
	require.Error(t, err)
}

func TestResolver_VaultNotCached_ManifoldFetchError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{
		GetArrowErr: vault.ErrNotCached,
	}
	m := &mocks.Manifold{
		ResolveArrowErr: errors.New("network error"),
	}

	r := newTestReaderWithVaultManifold(t, v, m)
	_, err := r.ResolveManifest(context.Background(), ns)
	require.Error(t, err)
}

func TestResolver_VaultNotCached_PutArrowError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Fresh"}}
	v := &mocks.Vault{
		GetArrowErr: vault.ErrNotCached,
		PutArrowErr: errors.New("disk full"),
	}
	m := &mocks.Manifold{
		ResolveArrowResult:   arrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}

	r := newTestReaderWithVaultManifold(t, v, m)
	_, err := r.ResolveManifest(context.Background(), ns)
	require.Error(t, err)
}

func TestResolver_VaultNotCached_IndexesResolvedManifest(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{
			Name:        "Chromium",
			Description: "A fast web browser",
			Tags:        []string{"browser"},
		},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64:  {},
			domain.OSDarwinARM64: {},
		},
	}
	dir := t.TempDir()
	v, err := vault.New(filepath.Join(dir, "vault"), filepath.Join(dir, "ns"), time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	m := &mocks.Manifold{
		ResolveArrowResult:   arrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}

	got, err := resolveViaManifest(t, v, m, ns)
	require.NoError(t, err)
	require.Equal(t, "Chromium", got.Name)

	rows, err := v.SearchArrows(context.Background(), vault.IndexQuery{Text: "chrom", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Chromium", rows[0].Meta.Arrow.Name)
	assert.Equal(t, []string{"browser"}, rows[0].Meta.Arrow.Tags)
	assert.ElementsMatch(t, []domain.OS{domain.OSLinuxAMD64, domain.OSDarwinARM64}, rows[0].Meta.OS)
}

func TestResolver_VaultStale_IndexesRefreshedManifest(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Chromium"},
		Targets:   map[domain.OS]domain.Target{domain.OSLinuxAMD64: {}},
	}
	dir := t.TempDir()
	base := time.Now()
	v, err := vault.NewWithClock(
		filepath.Join(dir, "vault"),
		filepath.Join(dir, "ns"),
		time.Hour,
		func() time.Time { return base },
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	m := &mocks.Manifold{
		ResolveArrowResult:   arrow,
		ResolveArrowRaw:      []byte("raw"),
		ResolveArrowFilename: "ARROW.md",
	}

	// Seed an unindexed entry, then age it past the TTL so the next resolve
	// takes the stale-refresh path.
	require.NoError(t, v.PutArrow(context.Background(), ns, vault.ManifestFile{
		Content: []byte("old"), Filename: "ARROW.md",
	}))

	staleVault, err := vault.NewWithClock(
		filepath.Join(dir, "vault"),
		filepath.Join(dir, "ns"),
		time.Hour,
		func() time.Time { return base.Add(2 * time.Hour) },
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = staleVault.Close() })

	_, err = resolveViaManifest(t, staleVault, m, ns)
	require.NoError(t, err)

	rows, err := staleVault.SearchArrows(context.Background(), vault.IndexQuery{Text: "chrom", Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestFetchAndCache_ManifoldNotFound_TranslatesToAppNotFound(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	m := &mocks.Manifold{ResolveArrowErr: fmt.Errorf("wrapped: %w", manifoldresolver.ErrNotFound)}

	_, err := resolveViaManifest(t, v, m, ns)
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrNotFound))
}

func TestFetchAndCache_ManifoldFetchFailed_TranslatesToAppFetchFailed(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	m := &mocks.Manifold{ResolveArrowErr: fmt.Errorf("wrapped: %w", manifoldresolver.ErrFetchFailed)}

	_, err := resolveViaManifest(t, v, m, ns)
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrFetchFailed))
}

func TestFetchAndCache_ManifoldInvalidManifest_TranslatesToAppInvalidManifest(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	m := &mocks.Manifold{ResolveArrowErr: fmt.Errorf("wrapped: %w", manifold.ErrInvalidManifest)}

	_, err := resolveViaManifest(t, v, m, ns)
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrInvalidManifest))
}

func TestFetchAndCache_ManifoldArrowNotInCollection_TranslatesToAppNotFound(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg/tool@v1.0.0")
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	m := &mocks.Manifold{ResolveArrowErr: fmt.Errorf("wrapped: %w", manifold.ErrArrowNotInCollection)}

	_, err := resolveViaManifest(t, v, m, ns)
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrNotFound))
}

func TestParseManifest_VaultHit_InvalidManifest_TranslatesToAppInvalidManifest(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{GetArrowFile: vault.ManifestFile{Content: []byte("raw")}}
	m := &mocks.Manifold{ParseArrowErr: fmt.Errorf("wrapped: %w", manifold.ErrInvalidManifest)}

	_, err := resolveViaManifest(t, v, m, ns)
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrInvalidManifest))
}

// A network error unrelated to manifest content must stay a generic failure,
// not be misclassified as an invalid manifest just because it isn't
// ErrNotFound or ErrFetchFailed either.
func TestFetchAndCache_UnrelatedManifoldError_NotClassifiedAsInvalidManifest(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	m := &mocks.Manifold{ResolveArrowErr: errors.New("network error")}

	_, err := resolveViaManifest(t, v, m, ns)
	require.Error(t, err)
	assert.False(t, errors.Is(err, apperrors.ErrInvalidManifest))
}

// TestFetchAndCache_ManifoldNotFound_CachesConfirmedAbsent proves the
// negative-caching fix at the mock-vault call-tracking level: a genuine
// ErrNotFound from the manifold triggers exactly one PutArrowNotFound call
// for the namespace that failed.
func TestFetchAndCache_ManifoldNotFound_CachesConfirmedAbsent(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	m := &mocks.Manifold{ResolveArrowErr: fmt.Errorf("wrapped: %w", manifoldresolver.ErrNotFound)}

	_, err := resolveViaManifest(t, v, m, ns)

	require.Error(t, err)
	require.Equal(t, 1, v.PutArrowNotFoundCalls)
	require.Len(t, v.PutArrowNotFoundNamespaces, 1)
	assert.Equal(t, ns, v.PutArrowNotFoundNamespaces[0])
}

// TestFetchAndCache_ManifoldFetchFailed_NeverCachedAsAbsent is the scope
// guard for the whole feature: a transient failure (network-style, not
// "not found") must never be recorded as confirmed absent — doing so would
// silently convert a temporary outage into a false not-found for a full
// TTL.
func TestFetchAndCache_ManifoldFetchFailed_NeverCachedAsAbsent(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	m := &mocks.Manifold{ResolveArrowErr: fmt.Errorf("wrapped: %w", manifoldresolver.ErrFetchFailed)}

	_, err := resolveViaManifest(t, v, m, ns)

	require.Error(t, err)
	assert.Zero(t, v.PutArrowNotFoundCalls)
}

// TestFetchAndCache_UnrelatedManifoldError_NeverCachedAsAbsent covers the
// same scope guard for a plain, unclassified error (no sentinel at all).
func TestFetchAndCache_UnrelatedManifoldError_NeverCachedAsAbsent(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}
	m := &mocks.Manifold{ResolveArrowErr: errors.New("network error")}

	_, err := resolveViaManifest(t, v, m, ns)

	require.Error(t, err)
	assert.Zero(t, v.PutArrowNotFoundCalls)
}

// TestFetchAndCache_PutArrowNotFoundError_StillReturnsTheNotFoundError
// proves a failure to write the negative-cache marker never masks the
// definitive not-found answer the manifold already gave.
func TestFetchAndCache_PutArrowNotFoundError_StillReturnsTheNotFoundError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{
		GetArrowErr:         vault.ErrNotCached,
		PutArrowNotFoundErr: errors.New("disk full"),
	}
	m := &mocks.Manifold{ResolveArrowErr: fmt.Errorf("wrapped: %w", manifoldresolver.ErrNotFound)}

	_, err := resolveViaManifest(t, v, m, ns)

	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrNotFound))
}

// TestResolveManifest_ConfirmedAbsent_SkipsManifoldEntirely is the actual
// regression guard for the reported bug: once a namespace is cached as
// confirmed absent, a second ResolveManifest call must not touch the
// manifold at all — no second clone, no second fetch attempt.
func TestResolveManifest_ConfirmedAbsent_SkipsManifoldEntirely(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{GetArrowErr: vault.ErrConfirmedAbsent}
	m := &mocks.Manifold{
		ResolveArrowErr: errors.New("must not be called: confirmed-absent cache hit should short-circuit"),
	}

	_, err := resolveViaManifest(t, v, m, ns)

	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrNotFound))
	assert.Zero(t, m.ResolveArrowCalls, "manifold.ResolveArrow must not be called on a confirmed-absent cache hit")
}

// TestResolveManifest_RealVault_ConfirmedAbsent_EndToEnd exercises the fix
// through a real vault.Vault instance (not the mock) end to end: the first
// resolution genuinely fetches (and fails) via the manifold; the second,
// immediately after, must be answered entirely from the vault's negative
// cache, without invoking the manifold again.
func TestResolveManifest_RealVault_ConfirmedAbsent_EndToEnd(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	dir := t.TempDir()
	v, err := vault.New(filepath.Join(dir, "vault"), filepath.Join(dir, "ns"), time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	m := &mocks.Manifold{ResolveArrowErr: fmt.Errorf("wrapped: %w", manifoldresolver.ErrNotFound)}

	_, err = resolveViaManifest(t, v, m, ns)
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrNotFound))
	firstCallCount := m.ResolveArrowCalls

	_, err = resolveViaManifest(t, v, m, ns)
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrNotFound))
	assert.Equal(t, firstCallCount, m.ResolveArrowCalls,
		"second resolution must be answered from the negative cache, not a second manifold call")
}

// TestResolveManifest_RealVault_ConfirmedAbsent_ExpiresAndRetries proves the
// TTL side of the fix: once the confirmed-absent marker's TTL has passed,
// resolution is attempted again — this matters for a mutable ref (a branch)
// that might gain a manifest later, even though it makes no difference for
// an immutable tag.
func TestResolveManifest_RealVault_ConfirmedAbsent_ExpiresAndRetries(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	dir := t.TempDir()
	base := time.Now()
	v, err := vault.NewWithClock(
		filepath.Join(dir, "vault"), filepath.Join(dir, "ns"), time.Hour,
		func() time.Time { return base },
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })
	m := &mocks.Manifold{ResolveArrowErr: fmt.Errorf("wrapped: %w", manifoldresolver.ErrNotFound)}

	_, err = resolveViaManifest(t, v, m, ns)
	require.Error(t, err)
	require.Equal(t, 1, m.ResolveArrowCalls)

	vAfterTTL, err := vault.NewWithClock(
		filepath.Join(dir, "vault"), filepath.Join(dir, "ns"), time.Hour,
		func() time.Time { return base.Add(2 * time.Hour) },
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = vAfterTTL.Close() })

	_, err = resolveViaManifest(t, vAfterTTL, m, ns)
	require.Error(t, err)
	assert.Equal(t, 2, m.ResolveArrowCalls,
		"a confirmed-absent marker past its TTL must be retried live, not trusted forever")
}

func TestResolveManifest_NoVault_FetchFromManifold_TranslatesNotFound(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	m := &mocks.Manifold{ResolveArrowErr: fmt.Errorf("wrapped: %w", manifoldresolver.ErrNotFound)}

	r := newTestReaderWithVaultManifold(t, nil, m)
	_, err := r.ResolveManifest(context.Background(), ns)
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrNotFound))
}
