package quiver

import (
	"context"
	"errors"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	quivercmds "github.com/rabbytesoftware/quiver/internal/app/quiver/commands"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestManifest(name string) *domain.QuiverManifest {
	return &domain.QuiverManifest{
		Name:        name,
		Description: "A test quiver",
		Tags:        []string{"test"},
	}
}

func makeQuiverServiceWithMocks(v vault.Vault, m *mocks.Manifold) *quiverService {
	return &quiverService{engines: engine.Container{Vault: v, Manifold: m}}
}

func testQuiverService(t *testing.T, v vault.Vault, m *mocks.Manifold) *quiverService {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)
	catalog, err := quiverstore.NewQuiverCatalog(":memory:")
	require.NoError(t, err)
	return &quiverService{
		asynxQuiver: axQuiver,
		catalog:     catalog,
		engines:     engine.Container{Vault: v, Manifold: m},
	}
}

func addQuiverForTest(t *testing.T, svc *quiverService, ns domain.Namespace, manifest *domain.QuiverManifest) {
	t.Helper()
	require.NoError(t, svc.catalog.Save(context.Background(), domain.Quiver{Namespace: ns, Manifest: *manifest}))
	_, err := svc.asynxQuiver.Send(context.Background(), quivercmds.AddQuiver{
		Namespace: ns,
		Manifest:  *manifest,
	})
	require.NoError(t, err)
	svc.asynxQuiver.WaitPublish()
}

// --- Add ---

func TestAdd_InvalidNamespace_ReturnsErrInvalidNamespace(t *testing.T) {
	svc := testQuiverService(t, &mocks.Vault{}, &mocks.Manifold{})

	err := svc.Add(context.Background(), "bad-namespace")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

func TestAdd_ManifoldFails_ReturnsErrFetchFailed(t *testing.T) {
	mv := &mocks.Vault{GetQuiverErr: vault.ErrNotCached}
	mm := &mocks.Manifold{ResolveQuiverErr: errors.New("network error")}
	svc := testQuiverService(t, mv, mm)

	err := svc.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchFailed)
}

func TestAdd_Success_QuiverSentToAsynx(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc := testQuiverService(t, mv, mm)

	err := svc.Add(context.Background(), "github.com/org/repo")
	require.NoError(t, err)

	svc.asynxQuiver.WaitPublish()

	got, err := svc.asynxQuiver.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), got.Namespace)
	assert.False(t, got.Removed)
}

func TestAdd_AlreadyExists_ReturnsError(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc := testQuiverService(t, mv, mm)

	require.NoError(t, svc.Add(context.Background(), "github.com/org/repo"))
	svc.asynxQuiver.WaitPublish()

	err := svc.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
}

// --- Update ---

func TestUpdate_NotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetQuiverErr: vault.ErrNotCached}
	mm := &mocks.Manifold{}
	svc := testQuiverService(t, mv, mm)

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUpdate_AlreadyRemoved_ReturnsErrAlreadyRemoved(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc := testQuiverService(t, mv, mm)

	addQuiverForTest(t, svc, "github.com/org/repo", manifest)

	// Mark as removed via command
	_, err := svc.asynxQuiver.Send(context.Background(), quivercmds.RemoveQuiver{Namespace: "github.com/org/repo"})
	require.NoError(t, err)
	svc.asynxQuiver.WaitPublish()

	err = svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyRemoved)
}

func TestUpdate_ManifoldFails_ReturnsErrFetchFailed(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc := testQuiverService(t, mv, mm)

	addQuiverForTest(t, svc, "github.com/org/repo", manifest)

	// Now make manifold fail
	mm.ResolveQuiverErr = errors.New("network error")
	mm.ResolveQuiverManifest = nil

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchFailed)
}

func TestUpdate_Success_UpdatesQuiver(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	updatedManifest := makeTestManifest("UpdatedQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc := testQuiverService(t, mv, mm)

	addQuiverForTest(t, svc, "github.com/org/repo", manifest)

	mm.ResolveQuiverManifest = updatedManifest
	err := svc.Update(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	svc.asynxQuiver.WaitPublish()

	got, err := svc.asynxQuiver.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, "UpdatedQuiver", got.Manifest.Name)
}

// --- Remove ---

func TestRemove_NotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetQuiverErr: vault.ErrNotCached}
	mm := &mocks.Manifold{}
	svc := testQuiverService(t, mv, mm)

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRemove_AlreadyRemoved_ReturnsErrAlreadyRemoved(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc := testQuiverService(t, mv, mm)

	addQuiverForTest(t, svc, "github.com/org/repo", manifest)

	// Remove once
	require.NoError(t, svc.Remove(context.Background(), "github.com/org/repo"))
	svc.asynxQuiver.WaitPublish()

	// Remove again — should fail
	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyRemoved)
}

func TestRemove_Success_QuiverMarkedRemoved(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc := testQuiverService(t, mv, mm)

	addQuiverForTest(t, svc, "github.com/org/repo", manifest)

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	svc.asynxQuiver.WaitPublish()

	got, err := svc.asynxQuiver.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.True(t, got.Removed)
}

// --- List ---

func TestList_EmptyCatalog_ReturnsEmpty(t *testing.T) {
	svc := testQuiverService(t, &mocks.Vault{}, &mocks.Manifold{})

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_FiltersRemovedQuivers(t *testing.T) {
	manifest := makeTestManifest("Quiver")
	svc := testQuiverService(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, svc.catalog.Save(context.Background(), domain.Quiver{
		Namespace: "github.com/org/active",
		Manifest:  *manifest,
		Removed:   false,
	}))
	require.NoError(t, svc.catalog.Save(context.Background(), domain.Quiver{
		Namespace: "github.com/org/removed",
		Manifest:  *manifest,
		Removed:   true,
	}))

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, domain.Namespace("github.com/org/active"), result[0].Namespace)
}

func TestList_IncludesManifestFields(t *testing.T) {
	svc := testQuiverService(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, svc.catalog.Save(context.Background(), domain.Quiver{
		Namespace: "github.com/org/repo",
		Manifest: domain.QuiverManifest{
			Name:        "TestQuiver",
			Description: "A test",
			Tags:        []string{"tag1", "tag2"},
		},
		Removed: false,
	}))

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "TestQuiver", result[0].Name)
	assert.Equal(t, "A test", result[0].Description)
	assert.Equal(t, []string{"tag1", "tag2"}, result[0].Tags)
}

// --- GetDetail ---

func TestGetDetail_NotFound_ReturnsErrNotFound(t *testing.T) {
	svc := testQuiverService(t, &mocks.Vault{}, &mocks.Manifold{})

	detail, err := svc.GetDetail(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, detail)
}

func TestGetDetail_Found_ReturnsDetailDTO(t *testing.T) {
	svc := testQuiverService(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, svc.catalog.Save(context.Background(), domain.Quiver{
		Namespace: ns,
		Manifest: domain.QuiverManifest{
			Name:        "TestQuiver",
			Description: "Desc",
			Tags:        []string{"a", "b"},
		},
		Removed: false,
	}))

	detail, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, ns, detail.Namespace)
	assert.Equal(t, "TestQuiver", detail.Manifest.Name)
	assert.Equal(t, "Desc", detail.Manifest.Description)
	assert.False(t, detail.Removed)
}

func TestGetDetail_RemovedQuiver_ReturnsRemovedTrue(t *testing.T) {
	svc := testQuiverService(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, svc.catalog.Save(context.Background(), domain.Quiver{
		Namespace: ns,
		Manifest:  domain.QuiverManifest{Name: "TestQuiver"},
		Removed:   true,
	}))

	detail, err := svc.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.True(t, detail.Removed)
}

// --- resolveManifest ---

func TestResolveManifest_FreshVaultHit_ReturnsManifestDirectly(t *testing.T) {
	manifest := makeTestManifest("FreshQuiver")
	mv := &mocks.Vault{
		GetQuiverEntry: &vault.QuiverVaultEntry{Manifest: manifest},
		GetQuiverPath:  "/home/fresh",
		GetQuiverErr:   nil,
	}
	mm := &mocks.Manifold{}
	svc := makeQuiverServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, manifest, got)
	assert.Equal(t, "/home/fresh", path)
	assert.Equal(t, 0, mv.PutQuiverCalls)
}

func TestResolveManifest_StaleVaultManifoldSucceeds_ReturnsFreshManifest(t *testing.T) {
	staleManifest := makeTestManifest("StaleQuiver")
	freshManifest := makeTestManifest("FreshQuiver")
	mv := &mocks.Vault{
		GetQuiverEntry: &vault.QuiverVaultEntry{Manifest: staleManifest},
		GetQuiverPath:  "/home/stale",
		GetQuiverErr:   vault.ErrStale,
		PutQuiverPath:  "/home/fresh",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: freshManifest}
	svc := makeQuiverServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, freshManifest, got)
	assert.Equal(t, "/home/fresh", path)
	assert.Equal(t, 1, mv.PutQuiverCalls)
}

func TestResolveManifest_StaleVaultManifoldFails_ReturnsStaleManifest(t *testing.T) {
	staleManifest := makeTestManifest("StaleQuiver")
	mv := &mocks.Vault{
		GetQuiverEntry: &vault.QuiverVaultEntry{Manifest: staleManifest},
		GetQuiverPath:  "/home/stale",
		GetQuiverErr:   vault.ErrStale,
	}
	mm := &mocks.Manifold{ResolveQuiverErr: errors.New("network error")}
	svc := makeQuiverServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, staleManifest, got)
	assert.Equal(t, "/home/stale", path)
	assert.Equal(t, 0, mv.PutQuiverCalls)
}

func TestResolveManifest_NotCachedManifoldSucceeds_ReturnsManifestAndStores(t *testing.T) {
	freshManifest := makeTestManifest("NewQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/home/new",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: freshManifest}
	svc := makeQuiverServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, freshManifest, got)
	assert.Equal(t, "/home/new", path)
	assert.Equal(t, 1, mv.PutQuiverCalls)
}

func TestResolveManifest_NotCachedManifoldFails_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{
		GetQuiverErr: vault.ErrNotCached,
	}
	mm := &mocks.Manifold{ResolveQuiverErr: errors.New("network error")}
	svc := makeQuiverServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

func TestResolveManifest_StaleVaultPutFails_ReturnsError(t *testing.T) {
	staleManifest := makeTestManifest("StaleQuiver")
	freshManifest := makeTestManifest("FreshQuiver")
	mv := &mocks.Vault{
		GetQuiverEntry: &vault.QuiverVaultEntry{Manifest: staleManifest},
		GetQuiverPath:  "/home/stale",
		GetQuiverErr:   vault.ErrStale,
		PutQuiverErr:   errors.New("disk full"),
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: freshManifest}
	svc := makeQuiverServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

func TestResolveManifest_NotCachedPutFails_ReturnsError(t *testing.T) {
	freshManifest := makeTestManifest("NewQuiver")
	mv := &mocks.Vault{
		GetQuiverErr: vault.ErrNotCached,
		PutQuiverErr: errors.New("disk full"),
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: freshManifest}
	svc := makeQuiverServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

func TestResolveManifest_UnexpectedVaultError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("disk failure")
	mv := &mocks.Vault{
		GetQuiverErr: unexpectedErr,
	}
	mm := &mocks.Manifold{}
	svc := makeQuiverServiceWithMocks(mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.ErrorIs(t, err, unexpectedErr)
	assert.Nil(t, got)
	assert.Empty(t, path)
}
