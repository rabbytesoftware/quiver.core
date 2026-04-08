package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog/store"
	quivercmds "github.com/rabbytesoftware/quiver/internal/app/quiver/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/domain"
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

func newAsynxQuiver(es asynxModels.Store) (asynx.Asynx[domain.Quiver], error) {
	return asynx.New[domain.Quiver]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

func testCatalog(
	t *testing.T,
	v vault.Vault,
	m *mocks.Manifold,
) (*catalogService, Catalog) {
	t.Helper()

	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	cat, err := store.NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	svc := &catalogService{
		axQuiver: axQuiver,
		store:    cat,
		vault:    v,
		manifold: m,
	}

	err = svc.registerProjections()
	require.NoError(t, err)

	return svc, svc
}

func seedQuiver(
	t *testing.T,
	svc *catalogService,
	ns domain.Namespace,
	manifest *domain.QuiverManifest,
	mv *mocks.Vault,
	mm *mocks.Manifold,
) {
	t.Helper()

	mv.GetQuiverErr = vault.ErrNotCached
	mv.PutQuiverPath = "/tmp/test"
	mm.ResolveQuiverManifest = manifest
	mm.ResolveQuiverErr = nil

	err := svc.Add(context.Background(), ns)
	require.NoError(t, err)
	svc.axQuiver.WaitPublish()
}

// --- Add ---

func TestAdd_InvalidNamespace_ReturnsErrInvalidNamespace(t *testing.T) {
	_, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	err := cat.Add(context.Background(), "bad-namespace")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidNamespace)
}

func TestAdd_ManifoldFails_ReturnsErrFetchFailed(t *testing.T) {
	mv := &mocks.Vault{GetQuiverErr: vault.ErrNotCached}
	mm := &mocks.Manifold{ResolveQuiverErr: errors.New("network error")}
	_, cat := testCatalog(t, mv, mm)

	err := cat.Add(context.Background(), "github.com/org/repo")
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
	svc, cat := testCatalog(t, mv, mm)

	err := cat.Add(context.Background(), "github.com/org/repo")
	require.NoError(t, err)

	svc.axQuiver.WaitPublish()

	got, err := svc.axQuiver.Get(context.Background(), "github.com/org/repo")
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
	svc, cat := testCatalog(t, mv, mm)

	require.NoError(t, cat.Add(context.Background(), "github.com/org/repo"))
	svc.axQuiver.WaitPublish()

	err := cat.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
}

// --- Update ---

func TestUpdate_NotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetQuiverErr: vault.ErrNotCached}
	mm := &mocks.Manifold{}
	_, cat := testCatalog(t, mv, mm)

	err := cat.Update(context.Background(), "github.com/org/repo")
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
	svc, cat := testCatalog(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	_, err := svc.axQuiver.Send(context.Background(), quivercmds.RemoveQuiver{Namespace: "github.com/org/repo"})
	require.NoError(t, err)
	svc.axQuiver.WaitPublish()

	err = cat.Update(context.Background(), "github.com/org/repo")
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
	svc, cat := testCatalog(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	mm.ResolveQuiverErr = errors.New("network error")
	mm.ResolveQuiverManifest = nil

	err := cat.Update(context.Background(), "github.com/org/repo")
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
	svc, cat := testCatalog(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	mm.ResolveQuiverManifest = updatedManifest
	err := cat.Update(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	svc.axQuiver.WaitPublish()

	got, err := svc.axQuiver.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, "UpdatedQuiver", got.Manifest.Name)
}

// --- Remove ---

func TestRemove_NotFound_ReturnsErrNotFound(t *testing.T) {
	mv := &mocks.Vault{GetQuiverErr: vault.ErrNotCached}
	mm := &mocks.Manifold{}
	_, cat := testCatalog(t, mv, mm)

	err := cat.Remove(context.Background(), "github.com/org/repo")
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
	svc, cat := testCatalog(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))
	svc.axQuiver.WaitPublish()

	err := cat.Remove(context.Background(), "github.com/org/repo")
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
	svc, cat := testCatalog(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	svc.axQuiver.WaitPublish()

	got, err := svc.axQuiver.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.True(t, got.Removed)
}

// --- List ---

func TestList_EmptyCatalog_ReturnsEmpty(t *testing.T) {
	_, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	result, err := cat.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_FiltersRemovedQuivers(t *testing.T) {
	manifest := makeTestManifest("Quiver")
	svc, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, svc.store.Save(context.Background(), domain.Quiver{
		Namespace: "github.com/org/active",
		Manifest:  *manifest,
		Removed:   false,
	}))
	require.NoError(t, svc.store.Save(context.Background(), domain.Quiver{
		Namespace: "github.com/org/removed",
		Manifest:  *manifest,
		Removed:   true,
	}))

	result, err := cat.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, domain.Namespace("github.com/org/active"), result[0].Namespace)
}

func TestList_IncludesManifestFields(t *testing.T) {
	svc, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, svc.store.Save(context.Background(), domain.Quiver{
		Namespace: "github.com/org/repo",
		Manifest: domain.QuiverManifest{
			Name:        "TestQuiver",
			Description: "A test",
			Tags:        []string{"tag1", "tag2"},
		},
		Removed: false,
	}))

	result, err := cat.List(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "TestQuiver", result[0].Manifest.Name)
	assert.Equal(t, "A test", result[0].Manifest.Description)
	assert.Equal(t, []string{"tag1", "tag2"}, result[0].Manifest.Tags)
}

// --- GetDetail ---

func TestGetDetail_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	detail, err := cat.GetDetail(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Nil(t, detail)
}

func TestGetDetail_Found_ReturnsQuiver(t *testing.T) {
	svc, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, svc.store.Save(context.Background(), domain.Quiver{
		Namespace: ns,
		Manifest: domain.QuiverManifest{
			Name:        "TestQuiver",
			Description: "Desc",
			Tags:        []string{"a", "b"},
		},
		Removed: false,
	}))

	detail, err := cat.GetDetail(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, ns, detail.Namespace)
	assert.Equal(t, "TestQuiver", detail.Manifest.Name)
	assert.False(t, detail.Removed)
}

func TestGetDetail_RemovedQuiver_ReturnsRemovedTrue(t *testing.T) {
	svc, cat := testCatalog(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, svc.store.Save(context.Background(), domain.Quiver{
		Namespace: ns,
		Manifest:  domain.QuiverManifest{Name: "TestQuiver"},
		Removed:   true,
	}))

	detail, err := cat.GetDetail(context.Background(), ns)
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
	svc, _ := testCatalog(t, mv, mm)

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
	svc, _ := testCatalog(t, mv, mm)

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
	svc, _ := testCatalog(t, mv, mm)

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
	svc, _ := testCatalog(t, mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, freshManifest, got)
	assert.Equal(t, "/home/new", path)
	assert.Equal(t, 1, mv.PutQuiverCalls)
}

func TestResolveManifest_NotCachedManifoldFails_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{GetQuiverErr: vault.ErrNotCached}
	mm := &mocks.Manifold{ResolveQuiverErr: errors.New("network error")}
	svc, _ := testCatalog(t, mv, mm)

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
	svc, _ := testCatalog(t, mv, mm)

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
	svc, _ := testCatalog(t, mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

func TestResolveManifest_UnexpectedVaultError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("disk failure")
	mv := &mocks.Vault{GetQuiverErr: unexpectedErr}
	mm := &mocks.Manifold{}
	svc, _ := testCatalog(t, mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.ErrorIs(t, err, unexpectedErr)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

// --- failingAxQuiver: stub Asynx that fails on the Nth Subscribe call ---

type failingAxQuiver struct {
	subscribeCallN int
	calls          int
	err            error
	getErr         error
	getResult      domain.Quiver
	sendErr        error
}

func (f *failingAxQuiver) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Quiver],
	_ ...asynxModels.SubscriptionOpt[domain.Quiver],
) (string, error) {
	f.calls++
	if f.calls == f.subscribeCallN {
		return "", f.err
	}
	return "sub-id", nil
}

func (f *failingAxQuiver) Send(_ context.Context, _ asynxModels.Command[domain.Quiver]) (asynxModels.Event[domain.Quiver], error) {
	return asynxModels.Event[domain.Quiver]{}, f.sendErr
}

func (f *failingAxQuiver) SendWait(_ context.Context, _ asynxModels.Command[domain.Quiver]) (asynxModels.Event[domain.Quiver], error) {
	return asynxModels.Event[domain.Quiver]{}, nil
}

func (f *failingAxQuiver) Shutdown(_ context.Context) error                { return nil }
func (f *failingAxQuiver) Get(_ context.Context, _ string) (domain.Quiver, error) {
	return f.getResult, f.getErr
}
func (f *failingAxQuiver) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *failingAxQuiver) Preload(_ context.Context, _ string) error         { return nil }
func (f *failingAxQuiver) Unsubscribe(_ string) error                        { return nil }
func (f *failingAxQuiver) Replay(_ context.Context, _ string, _ int64, _ int64, _ asynxModels.ProjectionHandler[domain.Quiver]) error {
	return nil
}
func (f *failingAxQuiver) WaitPublish() {}

// --- errStore: a mock QuiverCatalog that always errors ---

var errStoreFail = errors.New("store failure")

type errStore struct{}

func (e *errStore) Save(_ context.Context, _ domain.Quiver) error         { return errStoreFail }
func (e *errStore) Delete(_ context.Context, _ domain.Namespace) error    { return errStoreFail }
func (e *errStore) Get(_ context.Context, _ domain.Namespace) (*domain.Quiver, error) {
	return nil, errStoreFail
}
func (e *errStore) List(_ context.Context) ([]domain.Quiver, error) {
	return nil, errStoreFail
}

// --- registerProjections error paths ---

func TestRegisterProjections_FirstSubscribeFails_ReturnsError(t *testing.T) {
	stubErr := errors.New("subscribe error")
	ax := &failingAxQuiver{subscribeCallN: 1, err: stubErr}
	svc := &catalogService{axQuiver: ax, store: &errStore{}}

	err := svc.registerProjections()
	require.Error(t, err)
	assert.ErrorIs(t, err, stubErr)
}

func TestRegisterProjections_SecondSubscribeFails_ReturnsError(t *testing.T) {
	stubErr := errors.New("subscribe error 2")
	ax := &failingAxQuiver{subscribeCallN: 2, err: stubErr}
	svc := &catalogService{axQuiver: ax, store: &errStore{}}

	err := svc.registerProjections()
	require.Error(t, err)
	assert.ErrorIs(t, err, stubErr)
}

func TestRegisterProjections_ThirdSubscribeFails_ReturnsError(t *testing.T) {
	stubErr := errors.New("subscribe error 3")
	ax := &failingAxQuiver{subscribeCallN: 3, err: stubErr}
	svc := &catalogService{axQuiver: ax, store: &errStore{}}

	err := svc.registerProjections()
	require.Error(t, err)
	assert.ErrorIs(t, err, stubErr)
}

func TestNew_ProjectionsFail_ReturnsError(t *testing.T) {
	stubErr := errors.New("subscribe error")
	ax := &failingAxQuiver{subscribeCallN: 1, err: stubErr}

	cat, err := store.NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	_, err = New(ax, cat, &mocks.Vault{}, &mocks.Manifold{})
	require.Error(t, err)
}

// --- New ---

func TestNew_Success_ReturnsNonNilCatalog(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	cat, err := store.NewQuiverCatalog(":memory:")
	require.NoError(t, err)

	c, err := New(axQuiver, cat, &mocks.Vault{}, &mocks.Manifold{})
	require.NoError(t, err)
	assert.NotNil(t, c)
}

// --- Update error paths ---

func TestUpdate_VaultPutFails_ReturnsError(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, cat := testCatalog(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	// make vault PutQuiver fail for next call
	mv.PutQuiverErr = errors.New("vault full")
	mv.PutQuiverPath = ""
	mm.ResolveQuiverManifest = makeTestManifest("Updated")
	mm.ResolveQuiverErr = nil

	err := cat.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
}

// --- List error path ---

func TestList_StoreError_ReturnsError(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	svc := &catalogService{
		axQuiver: axQuiver,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
	}

	_, err = svc.List(context.Background())
	require.Error(t, err)
}

// --- GetDetail error path ---

func TestGetDetail_StoreError_ReturnsError(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	svc := &catalogService{
		axQuiver: axQuiver,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
	}

	_, err = svc.GetDetail(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
}

// --- Update / Remove error paths via failingAxQuiver ---

func TestUpdate_GetUnexpectedError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("unexpected get error")
	ax := &failingAxQuiver{getErr: unexpectedErr}
	svc := &catalogService{
		axQuiver: ax,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
	}

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, unexpectedErr)
}

func TestUpdate_SendFails_ReturnsError(t *testing.T) {
	sendErr := errors.New("send error")
	manifest := makeTestManifest("Quiver")
	ax := &failingAxQuiver{
		getResult: domain.Quiver{Namespace: "github.com/org/repo", Manifest: *manifest},
		sendErr:   sendErr,
	}
	svc := &catalogService{
		axQuiver: ax,
		store:    &errStore{},
		vault: &mocks.Vault{
			PutQuiverPath: "/tmp/test",
		},
		manifold: &mocks.Manifold{ResolveQuiverManifest: manifest},
	}

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, sendErr)
}

func TestRemove_GetUnexpectedError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("unexpected get error")
	ax := &failingAxQuiver{getErr: unexpectedErr}
	svc := &catalogService{
		axQuiver: ax,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
	}

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, unexpectedErr)
}

func TestRemove_SendFails_ReturnsError(t *testing.T) {
	sendErr := errors.New("send error")
	manifest := makeTestManifest("Quiver")
	ax := &failingAxQuiver{
		getResult: domain.Quiver{Namespace: "github.com/org/repo", Manifest: *manifest},
		sendErr:   sendErr,
	}
	svc := &catalogService{
		axQuiver: ax,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
	}

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, sendErr)
}
