package catalog

import (
	"context"
	"errors"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjections_ArrowAdded_UpdatesStore(t *testing.T) {
	m := makeManifest("Proj")
	svc, cat := testCatalog(t, resolveOK(m))

	ns := domain.Namespace("github.com/org/repo")

	require.NoError(t, cat.Add(context.Background(), ns))
	svc.axArrow.WaitPublish()

	// The store should have been populated by the projection.
	stored, err := svc.store.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, ns, stored.Namespace)
}

func TestProjections_ArrowUpdated_UpdatesStore(t *testing.T) {
	m := makeManifest("Proj")
	updated := makeManifest("Updated")
	calls := 0
	resolve := func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.ArrowManifest, error) {
		calls++
		if calls == 1 {
			return m, nil
		}
		return updated, nil
	}
	svc, cat := testCatalog(t, resolve)

	ns := domain.Namespace("github.com/org/repo")

	require.NoError(t, cat.Add(context.Background(), ns))
	svc.axArrow.WaitPublish()

	require.NoError(t, cat.Update(context.Background(), ns))
	svc.axArrow.WaitPublish()

	stored, err := svc.store.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, stored)
	var storedName string
	for _, mv := range stored.Versions {
		storedName = mv.Name
		break
	}
	assert.Equal(t, "Updated", storedName)
}

func TestProjections_ArrowRemoved_DeletesFromStore(t *testing.T) {
	m := makeManifest("Proj")
	svc, cat := testCatalog(t, resolveOK(m))

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

func TestNew_RegistersProjections_StorePopulated(t *testing.T) {
	m := makeManifest("NewTest")
	svc, cat := testCatalog(t, resolveOK(m))

	ns := domain.Namespace("github.com/org/new")

	require.NoError(t, cat.Add(context.Background(), ns))
	svc.axArrow.WaitPublish()

	stored, err := svc.store.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, ns, stored.Namespace)
}

// TestProjections_OnForget_StoreDeleteFails_ErrorSwallowed verifies that when the
// store's Delete returns an error during the OnForget callback the error is
// swallowed (only logged) and Remove still succeeds.
func TestProjections_OnForget_StoreDeleteFails_ErrorSwallowed(t *testing.T) {
	m := makeManifest("Proj")
	svc, cat := testCatalog(t, resolveOK(m))

	ns := domain.Namespace("github.com/org/repo")

	require.NoError(t, cat.Add(context.Background(), ns))
	svc.axArrow.WaitPublish()

	// Swap the store for one whose Delete always fails.
	svc.store = &failingArrowCatalog{deleteErr: errors.New("db unavailable")}

	// Remove triggers axArrow.Forget which fires the OnForget callback.
	// The callback calls store.Delete which fails; the warn path is covered.
	err := cat.Remove(context.Background(), ns)
	svc.axArrow.WaitPublish()

	// Remove itself must still succeed — Delete failure is best-effort / logged only.
	require.NoError(t, err)
}

func testCatalogWithAxArrow(
	t *testing.T,
	axArrow *failingAxArrow,
) error {
	t.Helper()

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	cat, err := store.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	_, newErr := New(axArrow, axRuntime, cat, nil)
	return newErr
}

func TestNew_ArrowAddedSubscribeFails_ReturnsError(t *testing.T) {
	subErr := assert.AnError
	fa := &failingAxArrow{subscribeCallN: 1, err: subErr}

	err := testCatalogWithAxArrow(t, fa)
	require.Error(t, err)
}

func TestNew_ArrowUpdatedSubscribeFails_ReturnsError(t *testing.T) {
	subErr := assert.AnError
	fa := &failingAxArrow{subscribeCallN: 2, err: subErr}

	err := testCatalogWithAxArrow(t, fa)
	require.Error(t, err)
}

func TestNew_OnForgetRegistrationFails_ReturnsError(t *testing.T) {
	wantErr := assert.AnError
	fa := &failingAxArrow{onForgetErr: wantErr}

	err := testCatalogWithAxArrow(t, fa)
	require.ErrorIs(t, err, wantErr)
}
