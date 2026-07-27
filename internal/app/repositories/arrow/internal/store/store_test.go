package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
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

func TestGetDetail_NotFound(t *testing.T) {
	r := newTestReader(t)
	_, err := r.GetDetail(context.Background(), domain.Namespace("github.com/nobody/pkg@v1"))
	require.Error(t, err)
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

func TestGetDetail_Found_WithRef_NotFound(t *testing.T) {
	r := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, r, domain.Arrow{Namespace: ns})

	// Request v2 which doesn't exist
	_, err := r.GetDetail(context.Background(), domain.Namespace("github.com/user/pkg@v2.0.0"))
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

	resolvedNs, got, constraint, err := r.ResolveForInstall(context.Background(), ns)
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

	resolvedNs, got, constraint, err := r.ResolveForInstall(context.Background(), glob)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", resolvedNs.Ref())
	assert.NotNil(t, got)
	assert.Equal(t, "v1.*", constraint)
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

	_, _, _, err := r.ResolveForInstall(context.Background(), glob)
	require.Error(t, err)
}

func TestResolveForInstall_ManifestError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{
		GetArrowErr: errors.New("vault error"),
	}
	m := &mocks.Manifold{}
	r := newTestReaderWithVaultManifold(t, v, m)

	_, _, _, err := r.ResolveForInstall(context.Background(), ns)
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

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, constraint, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
	)
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@v2.0.0"), resolvedNs)
	assert.NotNil(t, got)
	assert.Empty(t, constraint)
	assert.Equal(t, []domain.Namespace{"github.com/user/pkg@v2.0.0"}, *asked)
}

// The default branch is read off the remote, so a repository that defaults to
// neither main nor master resolves to the branch it actually has.
func TestResolveForInstall_Refless_NoStableRelease_TakesTheGitDefaultBranch(t *testing.T) {
	m, asked := branchServingManifold("develop")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable
	m.DefaultBranchRef = "develop"

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/char2cs/crowbar"),
	)
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/char2cs/crowbar@develop"), resolvedNs)
	require.NotNil(t, got)
	assert.Equal(t, "develop", got.Namespace.Ref())
	assert.Equal(t, []domain.Namespace{"github.com/char2cs/crowbar@develop"}, *asked)
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
	)
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@main"), resolvedNs)
	assert.NotNil(t, got)
	assert.Equal(t, []domain.Namespace{"github.com/user/pkg@main"}, *asked)
}

func TestResolveForInstall_Refless_TakesTheBranchThatServedTheManifest(t *testing.T) {
	m, asked := branchServingManifold("master")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
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

	r := newTestReaderWithVaultManifold(t, nil, m)

	_, _, _, err := r.ResolveForInstall(
		context.Background(),
		domain.Namespace("github.com/user/pkg"),
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
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, domain.Namespace("github.com/user/pkg@v1.2.3"), resolvedNs)
}

func TestResolveForInstall_Refless_VersionIsTheBranchThatServedIt(t *testing.T) {
	m, _ := branchServingManifold("master")
	m.ResolveLatestStableErr = manifold.ErrNoLatestStable

	r := newTestReaderWithVaultManifold(t, nil, m)

	resolvedNs, got, _, err := r.ResolveForInstall(context.Background(), domain.Namespace("github.com/user/pkg"))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "master", resolvedNs.Ref())
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
