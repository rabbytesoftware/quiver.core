package catalog

import (
	"context"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProjections_ArrowAdded verifies that an arrow.added event causes the
// store to be populated via the registered projection.
func TestProjections_ArrowAdded_UpdatesStore(t *testing.T) {
	manifest := makeManifest("Proj")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/org/repo")

	require.NoError(t, cat.Add(context.Background(), ns))
	svc.axArrow.WaitPublish()

	// The store should have been populated by the projection.
	stored, err := svc.store.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, ns, stored.Namespace)
}

// TestProjections_ArrowUpdated verifies that an arrow.updated event causes
// the store record to be refreshed.
func TestProjections_ArrowUpdated_UpdatesStore(t *testing.T) {
	manifest := makeManifest("Proj")
	updated := makeManifest("Updated")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/org/repo")

	require.NoError(t, cat.Add(context.Background(), ns))
	svc.axArrow.WaitPublish()

	mm.ResolveArrowManifest = updated
	require.NoError(t, cat.Update(context.Background(), ns))
	svc.axArrow.WaitPublish()

	stored, err := svc.store.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, stored)
	var storedName string
	for _, m := range stored.Versions {
		storedName = m.Name
		break
	}
	assert.Equal(t, "Updated", storedName)
}

// TestProjections_ArrowRemoved verifies that an arrow.removed event causes
// the store record to be deleted.
func TestProjections_ArrowRemoved_DeletesFromStore(t *testing.T) {
	manifest := makeManifest("Proj")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/org/repo")

	require.NoError(t, cat.Add(context.Background(), ns))
	svc.axArrow.WaitPublish()

	require.NoError(t, cat.Remove(context.Background(), ns))
	svc.axArrow.WaitPublish()

	// After removal the store record should be gone.
	stored, err := svc.store.Get(context.Background(), ns)
	require.NoError(t, err)
	assert.Nil(t, stored)
}

// TestNew_RegistersProjections ensures that New() wires up projections so that
// events flowing through axArrow are reflected in the store.
func TestNew_RegistersProjections_StorePopulated(t *testing.T) {
	manifest := makeManifest("NewTest")
	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveArrowManifest: manifest}

	svc, cat := testCatalog(t, mv, mm)

	ns := domain.Namespace("github.com/org/new")

	require.NoError(t, cat.Add(context.Background(), ns))
	svc.axArrow.WaitPublish()

	stored, err := svc.store.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, ns, stored.Namespace)
}

// --- New() with subscribe errors ---

func testCatalogWithAxArrow(
	t *testing.T,
	axArrow *failingAxArrow,
	v vault.Vault,
) error {
	t.Helper()

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	cat, err := store.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	_, newErr := New(axArrow, axRuntime, cat, v, &mocks.Manifold{})
	return newErr
}

func TestNew_ArrowAddedSubscribeFails_ReturnsError(t *testing.T) {
	subErr := assert.AnError
	fa := &failingAxArrow{subscribeCallN: 1, err: subErr}

	err := testCatalogWithAxArrow(t, fa, &mocks.Vault{})
	require.Error(t, err)
}

func TestNew_ArrowUpdatedSubscribeFails_ReturnsError(t *testing.T) {
	subErr := assert.AnError
	fa := &failingAxArrow{subscribeCallN: 2, err: subErr}

	err := testCatalogWithAxArrow(t, fa, &mocks.Vault{})
	require.Error(t, err)
}

func TestNew_OnForgetRegistrationFails_ReturnsError(t *testing.T) {
	wantErr := assert.AnError
	fa := &failingAxArrow{onForgetErr: wantErr}

	err := testCatalogWithAxArrow(t, fa, &mocks.Vault{})
	require.ErrorIs(t, err, wantErr)
}
