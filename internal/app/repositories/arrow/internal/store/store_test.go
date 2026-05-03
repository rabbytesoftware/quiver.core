package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	adapterSQLite "github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
)

func newTestReader(t *testing.T) (store.Store, asynx.Asynx[domain.Arrow]) {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	axArrow := newTestAsynxArrow(t)
	r, err := store.New(db, axArrow, nil, nil, nil)
	require.NoError(t, err)
	return r, axArrow
}

func newTestReaderWithRawDB(
	t *testing.T,
) (store.Store, *gormdb.DB, asynx.Asynx[domain.Arrow]) {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	axArrow := newTestAsynxArrow(t)
	r, err := store.New(db, axArrow, nil, nil, nil)
	require.NoError(t, err)
	return r, db, axArrow
}

func newTestReaderWithVaultManifold(
	t *testing.T,
	v vault.Vault,
	m *mocks.Manifold,
) (store.Store, asynx.Asynx[domain.Arrow]) {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	axArrow := newTestAsynxArrow(t)
	r, err := store.New(db, axArrow, v, m, nil)
	require.NoError(t, err)
	return r, axArrow
}

func newTestAsynxArrow(t *testing.T) asynx.Asynx[domain.Arrow] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

type addArrowCmd struct {
	arrow domain.Arrow
}

func (c addArrowCmd) AggregateID() string  { return c.arrow.Namespace.String() }
func (c addArrowCmd) EventName() string    { return "arrow.added." + c.arrow.Namespace.String() }
func (c addArrowCmd) ShouldSnapshot() bool { return true }
func (c addArrowCmd) Validate(current *domain.Arrow) error {
	if current != nil {
		return asynxModels.ErrValidation
	}
	return nil
}
func (c addArrowCmd) EmitEvent(_ *domain.Arrow) domain.Arrow { return c.arrow }

func seedArrow(t *testing.T, axArrow asynx.Asynx[domain.Arrow], arrow domain.Arrow) {
	t.Helper()
	_, err := axArrow.Send(context.Background(), addArrowCmd{arrow: arrow})
	require.NoError(t, err)
	axArrow.WaitPublish()
}

func TestList_Empty(t *testing.T) {
	r, _ := newTestReader(t)
	result, err := r.List(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_WithArrow(t *testing.T) {
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "My Pkg"},
	})

	result, err := r.List(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, ns.BareNamespace(), result[0].Namespace)
}

func TestList_FilterUserInstalled_True(t *testing.T) {
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{
		Namespace:     ns,
		UserInstalled: true,
	})

	trueVal := true
	result, err := r.List(context.Background(), &trueVal)
	require.NoError(t, err)
	assert.Len(t, result, 1)
}

func TestList_FilterUserInstalled_False(t *testing.T) {
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{
		Namespace:     ns,
		UserInstalled: true,
	})

	falseVal := false
	result, err := r.List(context.Background(), &falseVal)
	require.NoError(t, err)
	assert.Empty(t, result) // only user-installed arrows seeded
}

func TestGet_Found(t *testing.T) {
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Pkg"},
	})

	got, err := r.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Pkg", got.Name)
}

func TestGet_NotFound(t *testing.T) {
	r, _ := newTestReader(t)
	_, err := r.Get(context.Background(), domain.Namespace("github.com/nobody/pkg@v1"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGetDetail_NotFound(t *testing.T) {
	r, _ := newTestReader(t)
	_, err := r.GetDetail(context.Background(), domain.Namespace("github.com/nobody/pkg@v1"))
	require.Error(t, err)
}

func TestGetDetail_Found_NoRef(t *testing.T) {
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Detailed"},
	})

	got, err := r.GetDetail(context.Background(), ns.BareNamespace())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Detailed", got.Metadata.Name)
}

func TestGetDetail_Found_WithRef(t *testing.T) {
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Versioned"},
	})

	got, err := r.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Versioned", got.Metadata.Name)
}

func TestGetDetail_Found_WithRef_NotFound(t *testing.T) {
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{Namespace: ns})

	// Request v2 which doesn't exist
	_, err := r.GetDetail(context.Background(), domain.Namespace("github.com/user/pkg@v2.0.0"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGetManifest_Found_NoRef(t *testing.T) {
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Manifest"},
	})

	got, err := r.GetManifest(context.Background(), ns.BareNamespace())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Manifest", got.Name)
}

func TestGetManifest_Found_WithRef(t *testing.T) {
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Versioned Manifest"},
	})

	got, err := r.GetManifest(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestGetManifest_NotFound_BareNs(t *testing.T) {
	r, _ := newTestReader(t)
	_, err := r.GetManifest(context.Background(), domain.Namespace("github.com/nobody/pkg"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGetManifest_NotFound_SpecificRef(t *testing.T) {
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{Namespace: ns})

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

	r, _ := newTestReaderWithVaultManifold(t, v, m)

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

	r, _ := newTestReaderWithVaultManifold(t, v, m)

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

	r, _ := newTestReaderWithVaultManifold(t, v, m)

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

	r, _ := newTestReaderWithVaultManifold(t, v, m)

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

	axArrow := newTestAsynxArrow(t)
	_, err = store.New(db, axArrow, nil, nil, nil)
	require.Error(t, err)
}

func TestNew_ProjectionsError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	axArrow := newTestAsynxArrow(t)
	_ = axArrow.Shutdown(context.Background())

	_, err = store.New(db, axArrow, nil, nil, nil)
	require.Error(t, err)
}

// ─── DB-closed error paths ────────────────────────────────────────────────────

func TestList_DBError(t *testing.T) {
	r, db, _ := newTestReaderWithRawDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = r.List(context.Background(), nil)
	require.Error(t, err)
}

func TestGet_DBError(t *testing.T) {
	r, db, _ := newTestReaderWithRawDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = r.Get(context.Background(), domain.Namespace("github.com/user/pkg@v1.0.0"))
	require.Error(t, err)
}

func TestGetManifest_DBError(t *testing.T) {
	r, db, _ := newTestReaderWithRawDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = r.GetManifest(context.Background(), domain.Namespace("github.com/user/pkg@v1.0.0"))
	require.Error(t, err)
}

func TestGetDetail_DBError(t *testing.T) {
	r, db, _ := newTestReaderWithRawDB(t)
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
	r, _ := newTestReaderWithVaultManifold(t, v, m)

	_, _, _, err := r.ResolveForInstall(context.Background(), glob)
	require.Error(t, err)
}

func TestResolveForInstall_ManifestError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	v := &mocks.Vault{
		GetArrowErr: errors.New("vault error"),
	}
	m := &mocks.Manifold{}
	r, _ := newTestReaderWithVaultManifold(t, v, m)

	_, _, _, err := r.ResolveForInstall(context.Background(), ns)
	require.Error(t, err)
}

func TestList_WithUserInstalledFilter_False(t *testing.T) {
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{
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
	r, axArrow := newTestReader(t)
	ns := domain.Namespace("github.com/user/noinst@v1.0.0")
	seedArrow(t, axArrow, domain.Arrow{Namespace: ns})

	userInstalled := true
	result, err := r.List(context.Background(), &userInstalled)
	require.NoError(t, err)
	assert.Empty(t, result) // hasUserInstalled(versions) returns false
}
