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

func TestGetDetail_NotCatalogued_Refless_FollowsTheDefaultChannel(t *testing.T) {
	ns := domain.Namespace("github.com/char2cs/crowbar")
	m := &mocks.Manifold{
		SnapshotResult: domain.RefSnapshot{Branches: map[string]string{"develop": "d1"}, Head: "develop"},
		ResolveArrowAtCommitFn: func(_ context.Context, resolveNs domain.Namespace, _, commit string) (*domain.Arrow, []byte, string, error) {
			assert.Equal(t, "develop", resolveNs.Ref())
			assert.Equal(t, "d1", commit)
			return &domain.Arrow{Namespace: resolveNs, ArrowMeta: domain.ArrowMeta{Name: "Crowbar"}}, []byte("raw"), "ARROW.md", nil
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

// A preview answers "the way an add would": the selector kind it reports is
// the one the catalogued row would carry, not the zero value.
func TestGetDetail_UncataloguedPreview_ReportsTheSelectorKindAnAddWould(t *testing.T) {
	testCases := []struct {
		name    string
		ns      domain.Namespace
		snapErr error
		want    domain.SelectorKind
	}{
		{name: "rolling tag", ns: selectorBare.WithRef("nightly"), want: domain.SelectorChannel},
		{name: "exact tag", ns: selectorBare.WithRef("v1.2.0"), want: domain.SelectorPin},
		{name: "glob", ns: selectorBare.WithRef("v1.*"), want: domain.SelectorConstraint},
		{name: "remote unreadable", ns: selectorBare.WithRef("nightly"), snapErr: errors.New("ls-remote failed"), want: domain.SelectorPin},
		{name: "selector naming nothing", ns: selectorBare.WithRef("no-such-ref"), want: domain.SelectorPin},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			v := &mocks.Vault{GetArrowFile: vault.ManifestFile{Content: []byte("raw")}}
			m := &mocks.Manifold{
				ParseArrowResult: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "crowbar"}},
				SnapshotResult:   selectorSnapshot(),
				SnapshotErr:      tc.snapErr,
			}
			r := newTestReaderWithVaultManifold(t, v, m)

			got, err := r.GetDetail(context.Background(), tc.ns)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Metadata.SelectorKind)
		})
	}
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

func TestResolveManifest_BareNamespace_NotCatalogued_FollowsTheDefaultChannel(t *testing.T) {
	ns := domain.Namespace("github.com/user/newpkg")
	m := &mocks.Manifold{
		SnapshotResult: domain.RefSnapshot{Tags: map[string]string{"v3.0.0": "c3"}},
		ResolveArrowAtCommitFn: func(_ context.Context, resolveNs domain.Namespace, _, commit string) (*domain.Arrow, []byte, string, error) {
			assert.Equal(t, "c3", commit)
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

func TestResolveManifest_BareNamespace_NotCatalogued_StampsTheChannelIdentity(t *testing.T) {
	ns := domain.Namespace("github.com/user/newpkg")
	m := &mocks.Manifold{
		SnapshotResult: domain.RefSnapshot{Tags: map[string]string{"v3.0.0": "c3"}},
		ResolveArrowAtCommitFn: func(_ context.Context, resolveNs domain.Namespace, _, commit string) (*domain.Arrow, []byte, string, error) {
			assert.Equal(t, "c3", commit)
			return &domain.Arrow{Namespace: resolveNs, ArrowMeta: domain.ArrowMeta{Name: "New"}}, []byte("raw"), "ARROW.md", nil
		},
	}
	v := &mocks.Vault{GetArrowErr: vault.ErrNotCached}

	r := newTestReaderWithVaultManifold(t, v, m)
	got, err := r.ResolveManifest(context.Background(), ns)
	require.NoError(t, err)
	assert.Equal(t, ns.WithRef("stable"), got.Namespace)
	assert.Equal(t, domain.Resolved{Ref: "v3.0.0", Commit: "c3", Fingerprint: "c3"}, got.Resolved)
}

func TestResolveManifest_BareNamespace_DBError(t *testing.T) {
	r, db := newTestReaderWithRawDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = r.ResolveManifest(context.Background(), domain.Namespace("github.com/user/pkg"))
	require.Error(t, err)
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

// ─── channelOf ────────────────────────────────────────────────────────────
