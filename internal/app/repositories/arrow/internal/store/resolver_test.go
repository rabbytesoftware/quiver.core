package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
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
	r, _ := newTestReaderWithVaultManifold(t, v, m)
	return r.ResolveManifest(context.Background(), ns)
}

func TestResolver_NilVaultNilManifold_Error(t *testing.T) {
	r, _ := newTestReader(t)
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

	r, _ := newTestReaderWithVaultManifold(t, nil, m)
	got, err := r.ResolveManifest(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, "ManifoldOnly", got.Name)
}

func TestResolver_NilVault_ManifoldFetchError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	m := &mocks.Manifold{
		ResolveArrowErr: errors.New("manifold error"),
	}

	r, _ := newTestReaderWithVaultManifold(t, nil, m)
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
	axArrow := newTestAsynxArrow(t)
	r, err := store.New(db, axArrow, v, nil, nil) // nil manifold.Manifold interface
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

	r, _ := newTestReaderWithVaultManifold(t, v, m)
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
	axArrow := newTestAsynxArrow(t)
	r, err := store.New(db, axArrow, v, nil, nil) // nil manifold.Manifold
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

	r, _ := newTestReaderWithVaultManifold(t, v, m)
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

	r, _ := newTestReaderWithVaultManifold(t, v, m)
	_, err := r.ResolveManifest(context.Background(), ns)
	require.Error(t, err)
}
