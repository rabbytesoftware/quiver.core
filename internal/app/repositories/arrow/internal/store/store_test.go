package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func newTestReader(t *testing.T) store.Store {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	r, err := store.New(db, nil, nil)
	require.NoError(t, err)
	return r
}

func newTestReaderWithRawDB(
	t *testing.T,
) (store.Store, *gormdb.DB) {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	r, err := store.New(db, nil, nil)
	require.NoError(t, err)
	return r, db
}

func newTestReaderWithVaultManifold(
	t *testing.T,
	v vault.Vault,
	m *mocks.Manifold,
) store.Store {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	r, err := store.New(db, v, m)
	require.NoError(t, err)
	return r
}

func seedArrow(t *testing.T, r store.Store, arrow domain.Arrow) {
	t.Helper()
	require.NoError(t, r.Project(context.Background(), arrow))
}

func TestList_Empty(t *testing.T) {
	r := newTestReader(t)
	result, err := r.List(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_WithArrow(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "My Pkg"},
	})

	result, err := r.List(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, ns.BareNamespace(), result[0].Namespace)
}

func TestList_FilterUserInstalled_True(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{
		Namespace:     ns,
		UserInstalled: true,
	})

	trueVal := true
	result, err := r.List(context.Background(), &trueVal)
	require.NoError(t, err)
	assert.Len(t, result, 1)
}

func TestList_FilterUserInstalled_False(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{
		Namespace:     ns,
		UserInstalled: true,
	})

	falseVal := false
	result, err := r.List(context.Background(), &falseVal)
	require.NoError(t, err)
	assert.Empty(t, result) // only user-installed arrows seeded
}

func TestGet_Found(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Pkg"},
	})

	got, err := r.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Pkg", got.Name)
}

func TestGet_NotFound(t *testing.T) {
	r := newTestReader(t)
	_, err := r.Get(context.Background(), domain.Namespace("github.com/nobody/pkg@v1"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// GetDetail is a client's single entry point for "show me this arrow" —
// GetManifest/GetReadme already fall back to live resolution for a namespace
// nothing has catalogued, and GetDetail must agree instead of 404ing for a
// repository that genuinely exists.
func TestGetDetail_NotCatalogued_ExplicitRef_ResolvesLive(t *testing.T) {
	ns := domain.Namespace("github.com/char2cs/crowbar@nightly")
	arrow := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Crowbar Nightly"}}
	v := &mocks.Vault{GetArrowFile: vault.ManifestFile{Content: []byte("raw")}}
	m := &mocks.Manifold{ParseArrowResult: arrow}

	r := newTestReaderWithVaultManifold(t, v, m)

	got, err := r.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Crowbar Nightly", got.Metadata.Name)
	assert.Equal(t, ns, got.Metadata.Namespace)
	assert.Equal(t, domain.ArrowStateAbsent, got.State)
}

func TestGetDetail_NotCatalogued_Refless_FallsBackToLatestCascade(t *testing.T) {
	ns := domain.Namespace("github.com/char2cs/crowbar")
	m := &mocks.Manifold{
		ResolveLatestStableRef:    "develop",
		ResolveLatestInChannelRef: "develop",
		ResolveArrowFunc: func(_ context.Context, resolveNs domain.Namespace) (*domain.Arrow, []byte, string, error) {
			assert.Equal(t, "develop", resolveNs.Ref())
			return &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Crowbar"}}, []byte("raw"), "ARROW.md", nil
		},
	}
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r := newTestReaderWithVaultManifold(t, v, m)

	got, err := r.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Crowbar", got.Metadata.Name)
	assert.Equal(t, ns.WithRef("develop"), got.Metadata.Namespace)
	assert.Equal(t, domain.ArrowStateAbsent, got.State)
}

func TestGetDetail_NotCatalogued_ResolveNotFound_PropagatesNotFound(t *testing.T) {
	ns := domain.Namespace("github.com/nobody/pkg@v1")
	m := &mocks.Manifold{
		ResolveArrowFunc: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return nil, nil, "", manifoldresolver.ErrNotFound
		},
	}
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r := newTestReaderWithVaultManifold(t, v, m)

	_, err := r.GetDetail(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGetDetail_NotCatalogued_ResolveFetchFailed_PropagatesFetchFailed(t *testing.T) {
	ns := domain.Namespace("github.com/nobody/pkg@v1")
	m := &mocks.Manifold{
		ResolveArrowFunc: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return nil, nil, "", manifoldresolver.ErrFetchFailed
		},
	}
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r := newTestReaderWithVaultManifold(t, v, m)

	_, err := r.GetDetail(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

func TestGetDetail_Found_NoRef(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Detailed"},
	})

	got, err := r.GetDetail(context.Background(), ns.BareNamespace())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Detailed", got.Metadata.Name)
}

func TestGetDetail_Found_WithRef(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Versioned"},
	})

	got, err := r.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Versioned", got.Metadata.Name)
}

// A ref the catalog never added is not a reason to 404 a repository that
// does resolve — it falls back to the same live resolution an entirely
// uncatalogued namespace gets.
func TestGetDetail_CataloguedAtOtherRef_FallsBackToLiveResolve(t *testing.T) {
	catalogued := domain.Namespace("github.com/user/pkg@v1.0.0")
	requested := domain.Namespace("github.com/user/pkg@v2.0.0")
	arrow := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "V2"}}
	v := &mocks.Vault{GetArrowFile: vault.ManifestFile{Content: []byte("raw")}}
	m := &mocks.Manifold{ParseArrowResult: arrow}

	r := newTestReaderWithVaultManifold(t, v, m)
	seedArrow(t, r, domain.Arrow{Namespace: catalogued})

	got, err := r.GetDetail(context.Background(), requested)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "V2", got.Metadata.Name)
	assert.Equal(t, requested, got.Metadata.Namespace)
}

func TestGetDetail_CataloguedAtOtherRef_LiveResolveStillNotFound(t *testing.T) {
	catalogued := domain.Namespace("github.com/user/pkg@v1.0.0")
	requested := domain.Namespace("github.com/user/pkg@v2.0.0")
	m := &mocks.Manifold{
		ResolveArrowFunc: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return nil, nil, "", manifoldresolver.ErrNotFound
		},
	}
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r := newTestReaderWithVaultManifold(t, v, m)
	seedArrow(t, r, domain.Arrow{Namespace: catalogued})

	_, err := r.GetDetail(context.Background(), requested)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGetManifest_Found_NoRef(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Manifest"},
	})

	got, err := r.GetManifest(context.Background(), ns.BareNamespace())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Manifest", got.Name)
}

func TestGetManifest_Found_WithRef(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Versioned Manifest"},
	})

	got, err := r.GetManifest(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestGetManifest_NotFound_BareNs(t *testing.T) {
	r := newTestReader(t)
	_, err := r.GetManifest(context.Background(), domain.Namespace("github.com/nobody/pkg"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGetManifest_NotFound_SpecificRef(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{Namespace: ns})

	// Request v2 which doesn't exist
	_, err := r.GetManifest(context.Background(), domain.Namespace("github.com/user/pkg@v2.0.0"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestResolveManifest_Success(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Resolved"}}
	m := &mocks.Manifold{
		ParseArrowResult: arrow,
	}
	v := &mocks.Vault{
		GetArrowFile: vault.ManifestFile{Content: []byte("raw")},
	}

	r := newTestReaderWithVaultManifold(t, v, m)

	got, err := r.ResolveManifest(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Resolved", got.Name)
}

func TestResolveManifest_ParseError(t *testing.T) {
	m := &mocks.Manifold{
		ParseArrowErr: errors.New("parse failed"),
	}
	v := &mocks.Vault{
		GetArrowFile: vault.ManifestFile{Content: []byte("raw")},
	}

	r := newTestReaderWithVaultManifold(t, v, m)

	_, err := r.ResolveManifest(context.Background(), domain.Namespace("github.com/user/pkg@v1"))
	require.Error(t, err)
}

func TestResolveManifest_BareNamespace_CataloguedArrow_ResolvesAtInstalledRef(t *testing.T) {
	installedNs := domain.Namespace("github.com/char2cs/crowbar@v1.2.0")
	bareNs := installedNs.BareNamespace()

	m := &mocks.Manifold{
		ResolveArrowFunc: func(_ context.Context, ns domain.Namespace) (*domain.Arrow, []byte, string, error) {
			if ns.Ref() == "" {
				return nil, nil, "", errors.New("bare namespace resolved directly instead of via the catalogued ref")
			}
			return &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Crowbar"}}, []byte("raw"), "ARROW.md", nil
		},
	}
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r := newTestReaderWithVaultManifold(t, v, m)
	seedArrow(t, r, domain.Arrow{Namespace: installedNs, ArrowMeta: domain.ArrowMeta{Name: "Crowbar"}})

	got, err := r.ResolveManifest(context.Background(), bareNs)
	require.NoError(t, err)
	assert.Equal(t, "Crowbar", got.Name)
}

func TestResolveManifest_BareNamespace_NotCatalogued_FallsBackToLatestCascade(t *testing.T) {
	ns := domain.Namespace("github.com/user/newpkg")
	m := &mocks.Manifold{
		ResolveLatestStableRef:    "v3.0.0",
		ResolveLatestInChannelRef: "v3.0.0",
		ResolveArrowFunc: func(_ context.Context, resolveNs domain.Namespace) (*domain.Arrow, []byte, string, error) {
			assert.Equal(t, "v3.0.0", resolveNs.Ref())
			return &domain.Arrow{Namespace: resolveNs, ArrowMeta: domain.ArrowMeta{Name: "New"}}, []byte("raw"), "ARROW.md", nil
		},
	}
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r := newTestReaderWithVaultManifold(t, v, m)
	got, err := r.ResolveManifest(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, "New", got.Name)
}

func TestResolveManifest_ExplicitRef_Unchanged(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Pinned"}}
	v := &mocks.Vault{GetArrowFile: vault.ManifestFile{Content: []byte("raw")}}
	m := &mocks.Manifold{ParseArrowResult: arrow}

	r := newTestReaderWithVaultManifold(t, v, m)
	got, err := r.ResolveManifest(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, "Pinned", got.Name)
}

// A real manifest translation never sets domain.Arrow.Namespace (a manifest
// declares no version of its own), so these tests return it empty from the
// manifold mock — matching production behavior — to prove ResolveManifest
// stamps the resolved namespace itself rather than trusting the parsed arrow.

func TestResolveManifest_ExplicitRef_StampsNamespace(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Pinned"}}
	v := &mocks.Vault{GetArrowFile: vault.ManifestFile{Content: []byte("raw")}}
	m := &mocks.Manifold{ParseArrowResult: arrow}

	r := newTestReaderWithVaultManifold(t, v, m)
	got, err := r.ResolveManifest(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, ns, got.Namespace)
}

func TestResolveManifest_BareNamespace_CataloguedArrow_StampsInstalledRef(t *testing.T) {
	installedNs := domain.Namespace("github.com/char2cs/crowbar@v1.2.0")
	bareNs := installedNs.BareNamespace()

	m := &mocks.Manifold{
		ResolveArrowFunc: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "Crowbar"}}, []byte("raw"), "ARROW.md", nil
		},
	}
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r := newTestReaderWithVaultManifold(t, v, m)
	seedArrow(t, r, domain.Arrow{Namespace: installedNs, ArrowMeta: domain.ArrowMeta{Name: "Crowbar"}})

	got, err := r.ResolveManifest(context.Background(), bareNs)
	require.NoError(t, err)
	assert.Equal(t, installedNs, got.Namespace)
}

func TestResolveManifest_BareNamespace_NotCatalogued_StampsResolvedRef(t *testing.T) {
	ns := domain.Namespace("github.com/user/newpkg")
	m := &mocks.Manifold{
		ResolveLatestStableRef:    "v3.0.0",
		ResolveLatestInChannelRef: "v3.0.0",
		ResolveArrowFunc: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, []byte, string, error) {
			return &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "New"}}, []byte("raw"), "ARROW.md", nil
		},
	}
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r := newTestReaderWithVaultManifold(t, v, m)
	got, err := r.ResolveManifest(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, ns.WithRef("v3.0.0"), got.Namespace)
}

func TestResolveManifest_BareNamespace_DBError(t *testing.T) {
	r, db := newTestReaderWithRawDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = r.ResolveManifest(context.Background(), domain.Namespace("github.com/user/pkg"))
	require.Error(t, err)
}

func TestResolveForInstall_ExactRef(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := &domain.Arrow{Namespace: ns}
	m := &mocks.Manifold{
		ParseArrowResult: arrow,
	}
	v := &mocks.Vault{
		GetArrowFile: vault.ManifestFile{Content: []byte("raw")},
	}

	r := newTestReaderWithVaultManifold(t, v, m)

	resolvedNs, got, constraint, err := r.ResolveForInstall(context.Background(), ns, "")
	require.NoError(t, err)
	assert.Equal(t, ns, resolvedNs)
	assert.NotNil(t, got)
	assert.Equal(t, "", constraint) // not a glob → no constraint
}

func TestResolveForInstall_GlobRef(t *testing.T) {
	glob := domain.Namespace("github.com/user/pkg@v1.*")
	resolved := glob.BareNamespace().WithRef("v1.2.3")
	arrow := &domain.Arrow{Namespace: resolved}
	m := &mocks.Manifold{
		ResolveConstraintResult: "v1.2.3",
		ParseArrowResult:        arrow,
	}
	v := &mocks.Vault{
		GetArrowFile: vault.ManifestFile{Content: []byte("raw")},
	}

	r := newTestReaderWithVaultManifold(t, v, m)

	resolvedNs, got, constraint, err := r.ResolveForInstall(context.Background(), glob, "")
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", resolvedNs.Ref())
	assert.NotNil(t, got)
	assert.Equal(t, "v1.*", constraint)
	assert.False(t, got.RefIsBranch, "a tag resolved from a constraint is not a branch")
	assert.Empty(t, got.RefCommitSHA)
}

// ─── store.New error paths ──────────────────────────────────────────────────────

func TestNew_StorageError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = store.New(db, nil, nil)
	require.Error(t, err)
}

// ─── DB-closed error paths ────────────────────────────────────────────────────

func TestList_DBError(t *testing.T) {
	r, db := newTestReaderWithRawDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = r.List(context.Background(), nil)
	require.Error(t, err)
}

func TestGet_DBError(t *testing.T) {
	r, db := newTestReaderWithRawDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = r.Get(context.Background(), domain.Namespace("github.com/user/pkg@v1.0.0"))
	require.Error(t, err)
}

func TestGetManifest_DBError(t *testing.T) {
	r, db := newTestReaderWithRawDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = r.GetManifest(context.Background(), domain.Namespace("github.com/user/pkg@v1.0.0"))
	require.Error(t, err)
}

func TestGetDetail_DBError(t *testing.T) {
	r, db := newTestReaderWithRawDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = r.GetDetail(context.Background(), domain.Namespace("github.com/user/pkg@v1.0.0"))
	require.Error(t, err)
}

func TestResolveForInstall_GlobResolveError(t *testing.T) {
	glob := domain.Namespace("github.com/user/pkg@v1.*")
	m := &mocks.Manifold{
		ResolveConstraintErr: errors.New("constraint resolve error"),
	}
	v := &mocks.Vault{}
	r := newTestReaderWithVaultManifold(t, v, m)

	_, _, _, err := r.ResolveForInstall(context.Background(), glob, "")
	require.Error(t, err)
}

func TestResolveForInstall_ManifestError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{
		GetArrowErr: errors.New("vault error"),
	}
	m := &mocks.Manifold{}
	r := newTestReaderWithVaultManifold(t, v, m)

	_, _, _, err := r.ResolveForInstall(context.Background(), ns, "")
	require.Error(t, err)
}

func TestList_WithUserInstalledFilter_False(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{
		Namespace: ns,
		// UserInstalled = false (default)
	})

	userInstalled := true
	result, err := r.List(context.Background(), &userInstalled)
	require.NoError(t, err)
	// Arrow is NOT user-installed, so filter returns empty
	assert.Empty(t, result)
}

func TestHasUserInstalled_EmptyVersions_ReturnsFalse(t *testing.T) {
	// Test hasUserInstalled with empty versions via List filter
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/noinst@v1.0.0")
	seedArrow(t, r, domain.Arrow{Namespace: ns})

	userInstalled := true
	result, err := r.List(context.Background(), &userInstalled)
	require.NoError(t, err)
	assert.Empty(t, result) // hasUserInstalled(versions) returns false
}

func TestSearch_MatchesSeededArrow(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Searchable Pkg"},
	})

	got, err := r.Search(context.Background(), models.SearchQuery{Text: "Searchable"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, ns.BareNamespace(), got[0].Namespace)
	assert.Equal(t, "Searchable Pkg", got[0].Metadata.Name)
	assert.Equal(t, []string{"v1.0.0"}, got[0].Refs)
	assert.Equal(t, models.ProvenanceDependency, got[0].Provenance)
}

func TestSearch_NoMatchReturnsEmpty(t *testing.T) {
	r := newTestReader(t)
	seedArrow(t, r, domain.Arrow{
		Namespace: domain.Namespace("github.com/user/pkg@v1.0.0"),
		ArrowMeta: domain.ArrowMeta{Name: "Searchable Pkg"},
	})

	got, err := r.Search(context.Background(), models.SearchQuery{Text: "absent"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSearch_DBError(t *testing.T) {
	r, db := newTestReaderWithRawDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = r.Search(context.Background(), models.SearchQuery{Text: "anything"})
	require.Error(t, err)
}

// ─── ResolveForInstall: refless namespaces ───────────────────────────────────

// branchServingManifold answers ResolveArrow only for the refs listed in
// served, and records every namespace it was asked for, in order. It names no
// default branch unless a test sets one, which models a remote git cannot
// list — the only case that still reaches the configured branch list.
func branchServingManifold(
	served ...string,
) (*mocks.Manifold, *[]domain.Namespace) {
	asked := make([]domain.Namespace, 0, 4)
	m := &mocks.Manifold{}
	m.ResolveArrowFunc = func(
		_ context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, []byte, string, error) {
		asked = append(asked, ns)
		for _, ref := range served {
			if ns.Ref() == ref {
				return &domain.Arrow{Namespace: ns}, []byte("raw"), "arrow.yaml", nil
			}
		}
		return nil, nil, "", errors.New("not found")
	}
	return m, &asked
}

func TestResolveForInstall_Refless_ResolvesToLatestStable(t *testing.T) {
	m, asked := branchServingManifold("v2.0.0")
	m.ResolveLatestStableRef = "v2.0.0"
	m.ResolveLatestInChannelRef = "v2.0.0"

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, constraint, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@v2.0.0"), resolvedNs)
	assert.NotNil(t, got)
	assert.Empty(t, constraint)
	assert.Equal(t, []domain.Namespace{"github.com/user/pkg@v2.0.0"}, *asked)
	assert.False(t, got.RefIsBranch, "the latest stable release is a tag, not a branch")
	assert.Empty(t, got.RefCommitSHA)
}

// The default branch is read off the remote, so a repository that defaults to
// neither main nor master resolves to the branch it actually has.
func TestResolveForInstall_Refless_NoStableRelease_TakesTheGitDefaultBranch(t *testing.T) {
	m, asked := branchServingManifold("develop")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable
	m.DefaultBranchRef = "develop"
	m.DefaultBranchHash = "abc123def456"

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/char2cs/crowbar"),
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/char2cs/crowbar@develop"), resolvedNs)
	require.NotNil(t, got)
	assert.Equal(t, "develop", got.Namespace.Ref())
	assert.Equal(t, []domain.Namespace{"github.com/char2cs/crowbar@develop"}, *asked)
	assert.True(t, got.RefIsBranch, "resolved via the default-branch fallback: this is a mutable ref")
	assert.Equal(t, "abc123def456", got.RefCommitSHA)
}

// git answers for every host, so a domain the platform table has never heard of
// still resolves a refless namespace.
func TestResolveForInstall_Refless_UnknownPlatformResolvesOverGit(t *testing.T) {
	m, asked := branchServingManifold("trunk")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable
	m.DefaultBranchRef = "trunk"

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("git.example.invalid/user/pkg"),
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("git.example.invalid/user/pkg@trunk"), resolvedNs)
	assert.NotNil(t, got)
	assert.Equal(t, []domain.Namespace{"git.example.invalid/user/pkg@trunk"}, *asked)
}

// The branch git named is the answer: failing to read the manifest there is an
// error, not licence to try the configured list instead.
func TestResolveForInstall_Refless_GitDefaultBranchManifestErrorDoesNotFallBack(t *testing.T) {
	m, asked := branchServingManifold("main")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable
	m.DefaultBranchRef = "develop"

	r := newTestReaderWithVaultManifold(t, nil, m)

	_, _, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"",
	)
	require.Error(t, err)
	assert.Equal(t, []domain.Namespace{"github.com/user/pkg@develop"}, *asked)
}

// An unreachable remote cannot name a branch, but a raw fetch may still work,
// which is the whole remaining job of the configured list.
func TestResolveForInstall_Refless_UnreachableRemoteFallsBackToConfiguredList(t *testing.T) {
	m, asked := branchServingManifold("master")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable
	m.DefaultBranchErr = errors.New("dial tcp: connection refused")

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@master"), resolvedNs)
	assert.NotNil(t, got)
	assert.Equal(
		t,
		[]domain.Namespace{"github.com/user/pkg@main", "github.com/user/pkg@master"},
		*asked,
	)
}

func TestResolveForInstall_Refless_NoStableRelease_FallsBackToFirstDefaultBranch(t *testing.T) {
	m, asked := branchServingManifold("main", "master")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@main"), resolvedNs)
	assert.NotNil(t, got)
	assert.Equal(t, []domain.Namespace{"github.com/user/pkg@main"}, *asked)
	// The configured-branch list has no ls-remote step, so there is no hash to
	// stamp here — an accepted gap, not an oversight (see the design doc).
	assert.False(t, got.RefIsBranch)
	assert.Empty(t, got.RefCommitSHA)
}

func TestResolveForInstall_Refless_TakesTheBranchThatServedTheManifest(t *testing.T) {
	m, asked := branchServingManifold("master")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@master"), resolvedNs)
	assert.NotNil(t, got)
	assert.Equal(
		t,
		[]domain.Namespace{"github.com/user/pkg@main", "github.com/user/pkg@master"},
		*asked,
	)
}

func TestResolveForInstall_Refless_EmptyLatestStableRefFallsBack(t *testing.T) {
	m, _ := branchServingManifold("main")
	m.ResolveLatestStableRef = ""

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, _, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, "main", resolvedNs.Ref())
}

func TestResolveForInstall_Refless_NoBranchServesTheManifest(t *testing.T) {
	m, asked := branchServingManifold()
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable

	r := newTestReaderWithVaultManifold(t, nil, m)

	_, _, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"",
	)
	require.Error(t, err)
	assert.Len(t, *asked, 2)
}

// Unknown platform and an unlistable remote leaves nothing to resolve against.
func TestResolveForInstall_Refless_UnknownPlatformHasNoBranchToTry(t *testing.T) {
	m, asked := branchServingManifold("main")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable

	r := newTestReaderWithVaultManifold(t, nil, m)

	_, _, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("git.example.invalid/user/pkg"),
		"",
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Empty(t, *asked)
}

// A repository that publishes a stable release is answered by it: a manifest
// failure there is an error, not a reason to install the branch instead.
func TestResolveForInstall_Refless_LatestStableManifestErrorDoesNotFallBack(t *testing.T) {
	m, asked := branchServingManifold("main")
	m.ResolveLatestStableRef = "v2.0.0"
	m.ResolveLatestInChannelRef = "v2.0.0"

	r := newTestReaderWithVaultManifold(t, nil, m)

	_, _, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"",
	)
	require.Error(t, err)
	assert.Equal(t, []domain.Namespace{"github.com/user/pkg@v2.0.0"}, *asked)
}

func TestResolveForInstall_ExplicitRef_IsTakenAsWritten(t *testing.T) {
	m, asked := branchServingManifold("v1.0.0")
	m.ResolveLatestStableRef = "v9.9.9"

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, _, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg@v1.0.0"),
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@v1.0.0"), resolvedNs)
	assert.Equal(t, []domain.Namespace{"github.com/user/pkg@v1.0.0"}, *asked)
}

func TestResolveForInstall_GlobRef_ManifestError(t *testing.T) {
	m, _ := branchServingManifold()
	m.ResolveConstraintResult = "v1.2.3"

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, _, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg@v1.*"),
		"",
	)
	require.Error(t, err)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@v1.2.3"), resolvedNs)
}

// ─── the ref is the version ──────────────────────────────────────────────────

// Cached bytes carry whatever namespace they were parsed into, which is not a
// fact about where they were asked for. The ref the caller named is the arrow's
// version, so resolution has to hand that ref back untouched.
func TestResolveForInstall_ExplicitRef_IsTakenOverTheParsedManifest(t *testing.T) {
	m := &mocks.Manifold{
		ParseArrowResult: &domain.Arrow{Namespace: "github.com/user/pkg@nightly"},
	}
	v := &mocks.Vault{GetArrowFile: vault.ManifestFile{Content: []byte("raw")}}

	r := newTestReaderWithVaultManifold(t, v, m)

	resolvedNs, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg@v1.2.3"),
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@v1.2.3"), resolvedNs)
}

func TestResolveForInstall_Refless_VersionIsTheBranchThatServedIt(t *testing.T) {
	m, _ := branchServingManifold("master")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, _, err := r.ResolveForInstall(context.Background(), domain.Namespace("github.com/user/pkg"), "")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "master", resolvedNs.Ref())
}

// ─── ResolveForInstall: channel stamping ─────────────────────────────────────

func TestResolveForInstall_Refless_ChannelRequested_ResolvesInThatChannel(t *testing.T) {
	var capturedChannel string
	m := &mocks.Manifold{
		ResolveArrowResult: &domain.Arrow{Namespace: domain.Namespace("github.com/user/pkg@v1.5.0-rc2")},
	}
	m.ResolveLatestInChannelFn = func(_ context.Context, _ domain.Namespace, channel string) (string, error) {
		capturedChannel = channel
		return "v1.5.0-rc2", nil
	}
	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, resolvedArrow, constraint, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"rc",
	)
	require.NoError(t, err)
	assert.Equal(t, "", constraint)
	assert.Equal(t, "rc", resolvedArrow.Channel)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@v1.5.0-rc2"), resolvedNs)
	assert.Equal(t, "rc", capturedChannel, "the requested channel must be the one forwarded to ResolveLatestInChannel")
}

func TestResolveForInstall_Refless_NoChannelRequested_DefaultsToStable(t *testing.T) {
	var capturedChannel string
	m := &mocks.Manifold{
		ResolveArrowResult: &domain.Arrow{Namespace: domain.Namespace("github.com/user/pkg@v1.0.0")},
	}
	m.ResolveLatestInChannelFn = func(_ context.Context, _ domain.Namespace, channel string) (string, error) {
		capturedChannel = channel
		return "v1.0.0", nil
	}
	r := newTestReaderWithVaultManifold(t, nil, m)

	_, resolvedArrow, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, "stable", resolvedArrow.Channel)
	assert.Equal(t, manifold.StableChannel, capturedChannel, "an unspecified channel must default to stable at the manifold call site")
}

func TestResolveForInstall_ExplicitRef_ChannelDerivedFromRef(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.5.0-rc1")
	m := &mocks.Manifold{
		ParseArrowResult: &domain.Arrow{Namespace: ns},
	}
	v := &mocks.Vault{GetArrowFile: vault.ManifestFile{Content: []byte("raw")}}
	r := newTestReaderWithVaultManifold(t, v, m)

	_, resolvedArrow, _, err := r.ResolveForInstall(
		context.Background(),
		ns,
		"ignored-for-explicit-ref",
	)
	require.NoError(t, err)
	assert.Equal(t, "rc", resolvedArrow.Channel)
}

func TestResolveForInstall_GlobConstraint_ChannelDerivedFromResolvedTag(t *testing.T) {
	glob := domain.Namespace("github.com/user/pkg@v1.*")
	resolved := glob.BareNamespace().WithRef("v1.5.0-rc3")
	m := &mocks.Manifold{
		ResolveConstraintResult: "v1.5.0-rc3",
		ParseArrowResult:        &domain.Arrow{Namespace: resolved},
	}
	v := &mocks.Vault{GetArrowFile: vault.ManifestFile{Content: []byte("raw")}}
	r := newTestReaderWithVaultManifold(t, v, m)

	_, resolvedArrow, constraint, err := r.ResolveForInstall(
		context.Background(),
		glob,
		"",
	)
	require.NoError(t, err)
	assert.Equal(t, "v1.*", constraint)
	assert.Equal(t, "rc", resolvedArrow.Channel)
}

// TestResolveForInstall_Refless_DefaultBranch_RealChannelsElsewhere_DoesNotStampChannel
// is the regression guard for a real bug: a repository with genuine tags
// elsewhere (e.g. a single pointer-style tag, no stable release) must not
// have its default-branch fallback claim a Channel that never appears in
// its own ListChannels result — the arrow would otherwise show a Channel
// value with no matching option in its own channel dropdown.
func TestResolveForInstall_Refless_DefaultBranch_RealChannelsElsewhere_DoesNotStampChannel(t *testing.T) {
	m, _ := branchServingManifold("develop")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable
	m.DefaultBranchRef = "develop"
	m.DefaultBranchHash = "abc123def456"
	m.ListChannelsResult = []manifold.ChannelInfo{
		{Name: "nightly-rolling", Kind: "pointer", Latest: "nightly-rolling"},
	}

	r := newTestReaderWithVaultManifold(t, nil, m)

	_, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/char2cs/crowbar"),
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.RefIsBranch, "still a genuine branch-fallback resolution")
	assert.Empty(t, got.Channel,
		"must not claim \"develop\" as a tracked channel when \"nightly-rolling\" is the repo's only real, listed channel")
}

// TestResolveForInstall_Refless_DefaultBranch_NoTagsAtAll_StampsChannel is the
// control: a repository that genuinely publishes no tags at all still has
// its default branch as its one and only channel, and that case must keep
// stamping Channel exactly as before this fix.
func TestResolveForInstall_Refless_DefaultBranch_NoTagsAtAll_StampsChannel(t *testing.T) {
	m, _ := branchServingManifold("develop")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable
	m.DefaultBranchRef = "develop"
	m.DefaultBranchHash = "abc123def456"
	m.ListChannelsResult = []manifold.ChannelInfo{
		{Name: "develop", Kind: "pointer", Latest: "develop"},
	}

	r := newTestReaderWithVaultManifold(t, nil, m)

	_, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/char2cs/crowbar"),
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "develop", got.Channel,
		"a genuinely tag-less repo's default branch is its only channel, and must still be stamped")
}

// TestResolveForInstall_Refless_ConfiguredBranch_RealChannelsElsewhere_DoesNotStampChannel
// is resolveConfiguredBranch's counterpart of the DefaultBranch test above:
// the git-default-branch lookup itself is unavailable here, forcing the
// walk over the platform's configured branch list, but the same rule must
// still apply.
func TestResolveForInstall_Refless_ConfiguredBranch_RealChannelsElsewhere_DoesNotStampChannel(t *testing.T) {
	m, _ := branchServingManifold("main")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable
	m.ListChannelsResult = []manifold.ChannelInfo{
		{Name: "nightly-rolling", Kind: "pointer", Latest: "nightly-rolling"},
	}

	r := newTestReaderWithVaultManifold(t, nil, m)

	_, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.Channel,
		"must not claim \"main\" as a tracked channel when \"nightly-rolling\" is the repo's only real, listed channel")
}

// TestResolveForInstall_Refless_ConfiguredBranch_NoTagsAtAll_StampsChannel is
// the resolveConfiguredBranch control, mirroring the DefaultBranch one.
func TestResolveForInstall_Refless_ConfiguredBranch_NoTagsAtAll_StampsChannel(t *testing.T) {
	m, _ := branchServingManifold("main")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable
	m.ListChannelsResult = []manifold.ChannelInfo{
		{Name: "main", Kind: "pointer", Latest: "main"},
	}

	r := newTestReaderWithVaultManifold(t, nil, m)

	_, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "main", got.Channel,
		"a genuinely tag-less repo's default branch is its only channel, and must still be stamped")
}

// TestResolveForInstall_DefaultBranch_ListChannelsError_DoesNotStampChannel
// proves the fail-safe direction: a ListChannels error means "not
// confirmed", not "assume legitimate" — the branch-fallback resolution
// still succeeds, just without asserting an unverifiable channel.
func TestResolveForInstall_DefaultBranch_ListChannelsError_DoesNotStampChannel(t *testing.T) {
	m, _ := branchServingManifold("develop")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable
	m.DefaultBranchRef = "develop"
	m.DefaultBranchHash = "abc123def456"
	m.ListChannelsErr = errors.New("list tags unavailable")

	r := newTestReaderWithVaultManifold(t, nil, m)

	_, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/char2cs/crowbar"),
		"",
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.Channel)
}

// ─── Projection surface ──────────────────────────────────────────────────────

func TestProjectForget_RemovesTheVersion(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	arrow := domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "Pkg"}}

	require.NoError(t, r.Project(context.Background(), arrow))
	require.NoError(t, r.ProjectForget(context.Background(), arrow))

	_, err := r.Get(context.Background(), ns)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// ─── NeedsVersionCheck ───────────────────────────────────────────────────────

func newTestReaderWithClock(
	t *testing.T,
	clock func() time.Time,
) store.Store {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	r, err := store.NewWithClock(db, nil, nil, clock)
	require.NoError(t, err)
	return r
}

func newTestReaderWithClockAndRawDB(
	t *testing.T,
	clock func() time.Time,
) (store.Store, *gormdb.DB) {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	r, err := store.NewWithClock(db, nil, nil, clock)
	require.NoError(t, err)
	return r, db
}

func fixedClock(at time.Time) func() time.Time {
	return func() time.Time { return at }
}

// A namespace nothing has catalogued has no row to claim against, so there is
// nothing to check — this must be a plain "no" and never an error, so an
// uncatalogued/live-preview GetDetail never trips the check. A zero
// lastCheckedAt (as an uncatalogued read would report) is deliberately "as
// stale as it gets", so this still exercises the atomic fallback rather than
// short-circuiting.
func TestNeedsVersionCheck_NoCatalogRow_ReturnsFalse(t *testing.T) {
	r := newTestReaderWithClock(t, fixedClock(time.Now()))

	needs, err := r.NeedsVersionCheck(
		context.Background(), domain.Namespace("github.com/user/pkg@v1.0.0"), time.Time{},
	)
	require.NoError(t, err)
	assert.False(t, needs)
}

// A zero lastCheckedAt ("never checked", exactly what a fresh GetDetail read
// reports for a row that has never been claimed) is far outside the TTL, so
// this must fall through to the atomic claim and succeed.
func TestNeedsVersionCheck_NeverChecked_ClaimsAndReturnsTrue(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	r := newTestReaderWithClock(t, fixedClock(now))
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{Namespace: ns})

	needs, err := r.NeedsVersionCheck(context.Background(), ns, time.Time{})
	require.NoError(t, err)
	assert.True(t, needs, "never checked before is eligible")
}

// A lastCheckedAt within the TTL must be rejected by the in-memory pre-check
// alone — proven here by dropping the version table first: if the pre-check
// ever fell through to the atomic claim, this would error instead of
// answering false, since ClaimVersionCheck can no longer reach the table.
func TestNeedsVersionCheck_WithinTTL_NeverTouchesTheDatabase(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	r, db := newTestReaderWithClockAndRawDB(t, fixedClock(now))
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{Namespace: ns})

	require.NoError(t, db.Exec(`DROP TABLE catalog_arrow_versions`).Error)

	needs, err := r.NeedsVersionCheck(context.Background(), ns, now.Add(-time.Minute))
	require.NoError(t, err, "a call within the TTL must never reach the dropped table")
	assert.False(t, needs)
}

// The realistic shape of two GetDetail calls in quick succession: the second
// call re-fetches lastCheckedAt fresh (via GetDetail, exactly as
// arrowService.maybeCheckVersion does) and sees the stamp the first call's
// claim just wrote, so it must not claim again.
func TestNeedsVersionCheck_RecentlyClaimed_ReturnsFalseOnSecondCall(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	r := newTestReaderWithClock(t, fixedClock(now))
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{Namespace: ns})

	first, err := r.NeedsVersionCheck(context.Background(), ns, time.Time{})
	require.NoError(t, err)
	require.True(t, first)

	detail, err := r.GetDetail(context.Background(), ns)
	require.NoError(t, err)

	second, err := r.NeedsVersionCheck(context.Background(), ns, detail.LastVersionCheckAt)
	require.NoError(t, err)
	assert.False(t, second, "a stamp the first claim just wrote is not yet stale")
}

// ─── CheckVersionDrift: branch-tracked ───────────────────────────────────────

func branchTrackedArrow() domain.Arrow {
	return domain.Arrow{
		Namespace:    domain.Namespace("github.com/user/crowbar@develop"),
		RefIsBranch:  true,
		RefCommitSHA: "aaa111",
	}
}

func TestCheckVersionDrift_BranchTracked_TagNowExists_RecommendsIt(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveLatestStableRef: "v1.0.0",
	})

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), branchTrackedArrow())
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "v1.0.0", recommendedRef)
}

func TestCheckVersionDrift_BranchTracked_NoTags_SameBranchSameHash_NotOutdated(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveLatestStableErr: manifold.ErrNoLatestStable,
		DefaultBranchRef:       "develop",
		DefaultBranchHash:      "aaa111",
	})

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), branchTrackedArrow())
	require.True(t, ok)
	assert.False(t, outdated)
	assert.Empty(t, recommendedRef)
}

func TestCheckVersionDrift_BranchTracked_NoTags_HashMoved_Outdated(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveLatestStableErr: manifold.ErrNoLatestStable,
		DefaultBranchRef:       "develop",
		DefaultBranchHash:      "bbb222",
	})

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), branchTrackedArrow())
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Empty(t, recommendedRef, "a branch has no better named ref to switch to")
}

func TestCheckVersionDrift_BranchTracked_NoTags_DefaultBranchRenamed_Outdated(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveLatestStableErr: manifold.ErrNoLatestStable,
		DefaultBranchRef:       "main",
		DefaultBranchHash:      "aaa111",
	})

	outdated, _, ok := r.CheckVersionDrift(context.Background(), branchTrackedArrow())
	require.True(t, ok)
	assert.True(t, outdated)
}

func TestCheckVersionDrift_BranchTracked_LatestStableNetworkError_AbortsSilently(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveLatestStableErr: errors.New("dial tcp: connection refused"),
	})

	_, _, ok := r.CheckVersionDrift(context.Background(), branchTrackedArrow())
	assert.False(t, ok)
}

func TestCheckVersionDrift_BranchTracked_DefaultBranchNetworkError_AbortsSilently(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveLatestStableErr: manifold.ErrNoLatestStable,
		DefaultBranchErr:       errors.New("dial tcp: connection refused"),
	})

	_, _, ok := r.CheckVersionDrift(context.Background(), branchTrackedArrow())
	assert.False(t, ok)
}

// ─── CheckVersionDrift: tag-pinned ───────────────────────────────────────────

func TestCheckVersionDrift_TagPinned_ConstraintFindsNewerTag_Outdated(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveConstraintResult: "v1.5.0",
	})
	arrow := domain.Arrow{
		Namespace:           domain.Namespace("github.com/user/pkg@v1.0.0"),
		InstalledConstraint: "v1.*",
	}

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "v1.5.0", recommendedRef)
}

func TestCheckVersionDrift_TagPinned_ConstraintSameTag_NotOutdated(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveConstraintResult: "v1.0.0",
	})
	arrow := domain.Arrow{
		Namespace:           domain.Namespace("github.com/user/pkg@v1.0.0"),
		InstalledConstraint: "v1.*",
	}

	outdated, _, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.False(t, outdated)
}

func TestCheckVersionDrift_TagPinned_ExactPin_LatestStableFindsNewerTag_Outdated(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveLatestInChannelRef: "v2.0.0",
	})
	arrow := domain.Arrow{Namespace: domain.Namespace("github.com/user/pkg@v1.0.0")}

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "v2.0.0", recommendedRef)
}

func TestCheckVersionDrift_TagPinned_ChannelSet_UsesThatChannel(t *testing.T) {
	var capturedChannel string
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveLatestInChannelFn: func(
			_ context.Context,
			_ domain.Namespace,
			channel string,
		) (string, error) {
			capturedChannel = channel
			return "v1.5.0-rc2", nil
		},
	})
	arrow := domain.Arrow{
		Namespace: domain.Namespace("github.com/user/pkg@v1.5.0-rc1"),
		Channel:   "rc",
	}

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "v1.5.0-rc2", recommendedRef)
	assert.Equal(t, "rc", capturedChannel)
}

func TestCheckVersionDrift_TagPinned_EmptyChannel_DefaultsToStable_NotOutdated(t *testing.T) {
	var capturedChannel string
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveLatestInChannelFn: func(
			_ context.Context,
			_ domain.Namespace,
			channel string,
		) (string, error) {
			capturedChannel = channel
			return "v1.0.0", nil
		},
	})
	arrow := domain.Arrow{
		Namespace: domain.Namespace("github.com/user/pkg@v1.0.0"),
	}

	outdated, _, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.False(t, outdated)
	assert.Equal(t, manifold.StableChannel, capturedChannel)
}

func TestCheckVersionDrift_TagPinned_ConstraintTakesPriorityOverChannel(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveConstraintResult:   "v1.5.0",
		ResolveLatestInChannelRef: "v9.9.9",
	})
	arrow := domain.Arrow{
		Namespace:           domain.Namespace("github.com/user/pkg@v1.0.0"),
		InstalledConstraint: "v1.*",
		Channel:             "rc",
	}

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "v1.5.0", recommendedRef)
}

// TestCheckVersionDrift_TagPinned_PinnedRefTakesPriorityOverChannel proves
// ResolveTrackedRef's new pin priority (between constraint and channel-
// latest): a PinnedRef within a channel is what the drift check compares
// against, not the channel's own latest, and ResolveLatestInChannel must
// not even be consulted when a pin is set.
func TestCheckVersionDrift_TagPinned_PinnedRefTakesPriorityOverChannel(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveLatestInChannelFn: func(context.Context, domain.Namespace, string) (string, error) {
			t.Fatal("ResolveLatestInChannel must not run when a PinnedRef is set")
			return "", nil
		},
	})
	arrow := domain.Arrow{
		Namespace: domain.Namespace("github.com/user/pkg@v1.0.0"),
		Channel:   "beta",
		PinnedRef: "v1.1.0-beta.1",
	}

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "v1.1.0-beta.1", recommendedRef)
}

// TestCheckVersionDrift_TagPinned_PinnedRefMatchesInstalled_NotOutdated is
// PinnedRef's negative space: once the installed ref already is the pin,
// there is nothing to recommend.
func TestCheckVersionDrift_TagPinned_PinnedRefMatchesInstalled_NotOutdated(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{})
	arrow := domain.Arrow{
		Namespace: domain.Namespace("github.com/user/pkg@v1.0.0"),
		Channel:   "beta",
		PinnedRef: "v1.0.0",
	}

	outdated, _, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.False(t, outdated)
}

// TestCheckVersionDrift_TagPinned_ConstraintTakesPriorityOverPinnedRef proves
// ResolveTrackedRef's existing constraint-first rule still holds once a pin
// exists: InstalledConstraint and PinnedRef are documented as mutually
// exclusive in practice, but the resolution order itself must still put
// constraint ahead of a pin if both are somehow set.
func TestCheckVersionDrift_TagPinned_ConstraintTakesPriorityOverPinnedRef(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveConstraintResult: "v1.5.0",
	})
	arrow := domain.Arrow{
		Namespace:           domain.Namespace("github.com/user/pkg@v1.0.0"),
		InstalledConstraint: "v1.*",
		PinnedRef:           "v1.2.0",
	}

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "v1.5.0", recommendedRef, "the constraint's resolution must win over the pin")
}

// ─── channelOf ────────────────────────────────────────────────────────────

func TestChannelOf(t *testing.T) {
	testCases := []struct {
		name    string
		arrow   domain.Arrow
		wantChl string
	}{
		{
			name:    "EmptyChannel_DefaultsToStable",
			arrow:   domain.Arrow{},
			wantChl: manifold.StableChannel,
		},
		{
			name:    "NonEmptyChannel_PassesThroughUnchanged",
			arrow:   domain.Arrow{Channel: "rc"},
			wantChl: "rc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantChl, store.ChannelOf(tc.arrow))
		})
	}
}

func TestCheckVersionDrift_TagPinned_ExplicitRef_ResolveError_AbortsSilently(t *testing.T) {
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveConstraintErr: errors.New("dial tcp: connection refused"),
	})
	arrow := domain.Arrow{
		Namespace:           domain.Namespace("github.com/user/pkg@v1.0.0"),
		InstalledConstraint: "v1.*",
	}

	_, _, ok := r.CheckVersionDrift(context.Background(), arrow)
	assert.False(t, ok)
}

// TestCheckVersionDrift_BranchTrackedWithChannel_UsesChannelResolution guards
// the fix for an arrow that carries BOTH RefIsBranch (stamped by the
// refless-resolution fallback) AND a genuine, listed Channel on the same row
// -- resolveDefaultBranch stamps exactly this shape whenever the resolved
// branch also happens to be a real channel (e.g. quiver.desktop's own
// self-registration onto "nightly-rolling" before any stable release
// exists). Once a real Channel is set it must take priority over the legacy
// raw branch-hash comparison: checkBranchDrift only ever compares against
// the repository's default branch, blind to a DIFFERENT channel the user
// later picks (or a pin within one, per PinnedRef) -- it would otherwise
// silently keep comparing against the wrong branch forever.
func TestCheckVersionDrift_BranchTrackedWithChannel_UsesChannelResolution(t *testing.T) {
	// DefaultBranchRef/Hash are deliberately left zero: if this arrow were
	// wrongly routed through checkBranchDrift, ResolveDefaultBranch's zero
	// result would still report outdated (an empty branch never matches the
	// installed ref) but with an EMPTY recommendedRef -- checkBranchDrift can
	// never produce a non-empty one on its own. Asserting the channel's own
	// resolved ref below is therefore proof this went through
	// checkTagDrift/ResolveTrackedRef, not checkBranchDrift.
	r := newTestReaderWithVaultManifold(t, nil, &mocks.Manifold{
		ResolveLatestInChannelRef: "nightly-rolling-abc123",
	})
	arrow := domain.Arrow{
		Namespace:    domain.Namespace("github.com/user/crowbar@nightly-rolling-abc000"),
		RefIsBranch:  true,
		RefCommitSHA: "aaa111",
		Channel:      "nightly-rolling",
	}

	outdated, recommendedRef, ok := r.CheckVersionDrift(context.Background(), arrow)
	require.True(t, ok)
	assert.True(t, outdated)
	assert.Equal(t, "nightly-rolling-abc123", recommendedRef,
		"a Channel present must route through checkTagDrift, never checkBranchDrift")
}
