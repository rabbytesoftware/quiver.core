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

func TestResolveManifest_NoVault_FetchFromManifold_TranslatesNotFound(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	m := &mocks.Manifold{ResolveArrowErr: fmt.Errorf("wrapped: %w", manifoldresolver.ErrNotFound)}

	r := newTestReaderWithVaultManifold(t, nil, m)
	_, err := r.ResolveManifest(context.Background(), ns)
	require.Error(t, err)
	assert.True(t, errors.Is(err, apperrors.ErrNotFound))
}
