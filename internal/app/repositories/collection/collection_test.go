package collection

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/collection/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func makeTestManifest(name string) *domain.Collection {
	return &domain.Collection{
		Meta: domain.CollectionMeta{
			Name:        name,
			Description: "A test quiver",
			Tags:        []string{"test"},
		},
	}
}

func newAsynxCollection(
	es asynxModels.Store,
	ss asynxModels.SnapshotStore,
) (asynx.Asynx[domain.Collection], error) {
	return asynx.New[domain.Collection]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

func testRepository(
	t *testing.T,
	v vault.Vault,
	m *mocks.Manifold,
) (*collectionService, Collection) {
	t.Helper()

	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)

	axCollection, err := newAsynxCollection(es, ss)
	require.NoError(t, err)

	s, err := store.New(":memory:")
	require.NoError(t, err)

	svc := &collectionService{
		axCollection: axCollection,
		store:        s,
		vault:        v,
		manifold:     m,
	}

	require.NoError(t, svc.registerProjections())

	return svc, svc
}

func seedCollection(
	t *testing.T,
	svc *collectionService,
	ns domain.Namespace,
) {
	t.Helper()
	require.NoError(t, svc.Follow(context.Background(), ns, &domain.Collection{}, nil))
	svc.axCollection.WaitPublish()
}

// --- Follow ---

func TestFollow_InvalidNamespace_ReturnsErrInvalidNamespace(t *testing.T) {
	_, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	err := repo.Follow(context.Background(), "bad-namespace", &domain.Collection{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestFollow_ErrorsOnDuplicate(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, repo.Follow(context.Background(), "github.com/org/repo", &domain.Collection{}, nil))
	svc.axCollection.WaitPublish()

	err := repo.Follow(context.Background(), "github.com/org/repo", &domain.Collection{}, nil)
	require.Error(t, err)
}

func TestFollow_StoresFollowState(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, repo.Follow(context.Background(), "github.com/org/repo", &domain.Collection{}, nil))
	svc.axCollection.WaitPublish()

	got, err := svc.axCollection.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), got.Namespace)
}

func TestFollow_SendGenericError_ReturnsError(t *testing.T) {
	sendErr := errors.New("send error")
	ax := &failingAxQuiver{sendErr: sendErr}
	svc := &collectionService{axCollection: ax, store: &errStore{}, vault: &mocks.Vault{}, manifold: &mocks.Manifold{}}

	err := svc.Follow(context.Background(), "github.com/org/repo@v1.0.0", &domain.Collection{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, sendErr)
}

// --- Unfollow ---

func TestUnfollow_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	err := repo.Unfollow(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUnfollow_Success_ForgetsAggregateFromAsynx(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	seedCollection(t, svc, ns)

	require.NoError(t, repo.Unfollow(context.Background(), ns))

	exists, err := svc.axCollection.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestUnfollow_AfterUnfollowed_ReturnsErrNotFound(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	seedCollection(t, svc, "github.com/org/repo")

	require.NoError(t, repo.Unfollow(context.Background(), "github.com/org/repo"))

	err := repo.Unfollow(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUnfollow_ExistsUnexpectedError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("unexpected exists error")
	ax := &failingAxQuiver{existsErr: unexpectedErr}
	svc := &collectionService{
		axCollection: ax,
		store:        &errStore{},
		vault:        &mocks.Vault{},
		manifold:     &mocks.Manifold{},
	}

	err := svc.Unfollow(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, unexpectedErr)
}

func TestUnfollow_ForgetFails_ReturnsError(t *testing.T) {
	forgetErr := errors.New("forget error")
	ax := &failingAxQuiver{
		existsResult: true,
		forgetErr:    forgetErr,
	}
	svc := &collectionService{
		axCollection: ax,
		store:        &errStore{},
		vault:        &mocks.Vault{},
		manifold:     &mocks.Manifold{},
	}

	err := svc.Unfollow(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, forgetErr)
}

// --- List ---

func TestList_EmptyRepository_ReturnsEmpty(t *testing.T) {
	_, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	result, err := repo.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_ReturnsAllEntries(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, svc.store.Save(context.Background(), domain.Collection{
		Namespace: "github.com/org/one",
	}))
	require.NoError(t, svc.store.Save(context.Background(), domain.Collection{
		Namespace: "github.com/org/two",
	}))

	result, err := repo.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// --- List error path ---

func TestList_StoreError_ReturnsError(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)

	axCollection, err := newAsynxCollection(es, ss)
	require.NoError(t, err)

	svc := &collectionService{
		axCollection: axCollection,
		store:        &errStore{},
		vault:        &mocks.Vault{},
		manifold:     &mocks.Manifold{},
	}

	_, err = svc.List(context.Background())
	require.Error(t, err)
}

// --- Get (vault cache-or-fetch) ---

func TestGet_FreshVaultHit_ReturnsManifest(t *testing.T) {
	manifest := makeTestManifest("FreshQuiver")
	mv := &mocks.Vault{
		GetCollectionEntry: &vault.CollectionVaultEntry{Collection: manifest},
		GetCollectionPath:  "/home/fresh",
		GetCollectionErr:   nil,
	}
	mm := &mocks.Manifold{}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, manifest, got)

	assert.Equal(t, 0, mv.PutCollectionCalls)
}

func TestGet_StaleVaultManifoldSucceeds_ReturnsFreshManifest(t *testing.T) {
	staleManifest := makeTestManifest("StaleQuiver")
	freshManifest := makeTestManifest("FreshQuiver")
	mv := &mocks.Vault{
		GetCollectionEntry: &vault.CollectionVaultEntry{Collection: staleManifest},
		GetCollectionPath:  "/home/stale",
		GetCollectionErr:   vault.ErrStale,
		PutCollectionPath:  "/home/fresh",
	}
	mm := &mocks.Manifold{ResolveCollectionResult: freshManifest}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, freshManifest, got)

	assert.Equal(t, 1, mv.PutCollectionCalls)
}

func TestGet_StaleVaultManifoldFails_ReturnsStaleManifest(t *testing.T) {
	staleManifest := makeTestManifest("StaleQuiver")
	mv := &mocks.Vault{
		GetCollectionEntry: &vault.CollectionVaultEntry{Collection: staleManifest},
		GetCollectionPath:  "/home/stale",
		GetCollectionErr:   vault.ErrStale,
	}
	mm := &mocks.Manifold{ResolveCollectionErr: errors.New("network error")}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, staleManifest, got)

	assert.Equal(t, 0, mv.PutCollectionCalls)
}

func TestGet_NotCachedManifoldSucceeds_ReturnsManifestAndStores(t *testing.T) {
	freshManifest := makeTestManifest("NewQuiver")
	mv := &mocks.Vault{
		GetCollectionErr:  vault.ErrNotCached,
		PutCollectionPath: "/home/new",
	}
	mm := &mocks.Manifold{ResolveCollectionResult: freshManifest}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, freshManifest, got)

	assert.Equal(t, 1, mv.PutCollectionCalls)
}

func TestGet_NotCachedManifoldFails_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{GetCollectionErr: vault.ErrNotCached}
	mm := &mocks.Manifold{ResolveCollectionErr: errors.New("network error")}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestGet_UnexpectedVaultError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("disk failure")
	mv := &mocks.Vault{GetCollectionErr: unexpectedErr}
	mm := &mocks.Manifold{}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.ErrorIs(t, err, unexpectedErr)
	assert.Nil(t, got)
}

func TestGet_StaleVault_PutFails_ReturnsError(t *testing.T) {
	staleManifest := makeTestManifest("StaleQuiver")
	freshManifest := makeTestManifest("FreshQuiver")
	putErr := errors.New("put error")
	mv := &mocks.Vault{
		GetCollectionEntry: &vault.CollectionVaultEntry{Collection: staleManifest},
		GetCollectionPath:  "/home/stale",
		GetCollectionErr:   vault.ErrStale,
		PutCollectionErr:   putErr,
	}
	mm := &mocks.Manifold{ResolveCollectionResult: freshManifest}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, putErr)
	assert.Nil(t, got)
}

func TestGet_NotCached_PutFails_ReturnsError(t *testing.T) {
	freshManifest := makeTestManifest("FreshQuiver")
	putErr := errors.New("put error")
	mv := &mocks.Vault{
		GetCollectionErr: vault.ErrNotCached,
		PutCollectionErr: putErr,
	}
	mm := &mocks.Manifold{ResolveCollectionResult: freshManifest}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, putErr)
	assert.Nil(t, got)
}

// --- IsFollowed ---

func TestIsFollowed_WhenFollowed_ReturnsTrue(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	seedCollection(t, svc, "github.com/org/repo")

	followed, err := repo.IsFollowed(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.True(t, followed)
}

func TestIsFollowed_WhenNotFollowed_ReturnsFalse(t *testing.T) {
	_, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	followed, err := repo.IsFollowed(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.False(t, followed)
}

// --- OnCollectionFollowed ---

func TestOnCollectionFollowed_CallbackFires_OnFollow(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	var fired atomic.Bool
	require.NoError(t, repo.OnCollectionFollowed(func(_ context.Context, q domain.Collection) {
		fired.Store(true)
	}))

	require.NoError(t, repo.Follow(context.Background(), "github.com/org/repo", &domain.Collection{}, nil))
	svc.axCollection.WaitPublish()

	assert.True(t, fired.Load())
}

// --- OnCollectionUnfollowed ---

func TestOnCollectionUnfollowed_CallbackFires_OnUnfollow(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	seedCollection(t, svc, ns)

	var captured domain.Namespace
	require.NoError(t, repo.OnCollectionUnfollowed(func(_ context.Context, removed domain.Namespace) {
		captured = removed
	}))

	require.NoError(t, repo.Unfollow(context.Background(), ns))
	svc.axCollection.WaitPublish()

	assert.Equal(t, ns, captured)
}

// --- New constructor ---

func TestNew_Success_ReturnsNonNilRepository(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)

	axCollection, err := newAsynxCollection(es, ss)
	require.NoError(t, err)

	s, err := store.New(":memory:")
	require.NoError(t, err)

	repo, err := New(axCollection, s, &mocks.Vault{}, &mocks.Manifold{})
	require.NoError(t, err)
	assert.NotNil(t, repo)
}

func TestNew_ProjectionsFail_ReturnsError(t *testing.T) {
	stubErr := errors.New("subscribe error")
	ax := &failingAxQuiver{subscribeCallN: 1, err: stubErr}

	s, err := store.New(":memory:")
	require.NoError(t, err)

	_, err = New(ax, s, &mocks.Vault{}, &mocks.Manifold{})
	require.Error(t, err)
}

// --- failingAxQuiver: stub Asynx that fails on the Nth Subscribe call ---

type failingAxQuiver struct {
	subscribeCallN int
	calls          int
	err            error
	getErr         error
	getResult      domain.Collection
	existsErr      error
	existsResult   bool
	sendErr        error
	forgetErr      error
	onForgetErr    error
	shutdownErr    error
}

func (f *failingAxQuiver) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Collection],
	_ ...asynxModels.SubscriptionOpt[domain.Collection],
) (string, error) {
	f.calls++
	if f.calls == f.subscribeCallN {
		return "", f.err
	}
	return "sub-id", nil
}

func (f *failingAxQuiver) Send(_ context.Context, _ asynxModels.Command[domain.Collection]) (asynxModels.Event[domain.Collection], error) {
	return asynxModels.Event[domain.Collection]{}, f.sendErr
}

func (f *failingAxQuiver) SendWait(_ context.Context, _ asynxModels.Command[domain.Collection]) (asynxModels.Event[domain.Collection], error) {
	return asynxModels.Event[domain.Collection]{}, nil
}

func (f *failingAxQuiver) Shutdown(_ context.Context) error { return f.shutdownErr }
func (f *failingAxQuiver) Get(_ context.Context, _ string) (domain.Collection, error) {
	return f.getResult, f.getErr
}

func (f *failingAxQuiver) Exists(_ context.Context, _ string) (bool, error) {
	return f.existsResult, f.existsErr
}
func (f *failingAxQuiver) Preload(_ context.Context, _ string) error { return nil }
func (f *failingAxQuiver) Unsubscribe(_ string) error                { return nil }
func (f *failingAxQuiver) Replay(_ context.Context, _ string, _, _ int64, _ asynxModels.ProjectionHandler[domain.Collection]) error {
	return nil
}
func (f *failingAxQuiver) WaitPublish()                             {}
func (f *failingAxQuiver) Forget(_ context.Context, _ string) error { return f.forgetErr }
func (f *failingAxQuiver) OnForget(_ asynxModels.ForgetHandler[domain.Collection]) (string, error) {
	if f.onForgetErr != nil {
		return "", f.onForgetErr
	}
	return "forget-sub-id", nil
}

func (f *failingAxQuiver) Listen(_ string, _ int) (<-chan asynxModels.Event[domain.Collection], func(), error) {
	return nil, func() {}, nil
}

func (f *failingAxQuiver) SubscribeWait(_ context.Context, _ string) (asynxModels.Event[domain.Collection], error) {
	return asynxModels.Event[domain.Collection]{}, nil
}

// --- errStore ---

var errStoreFail = errors.New("store failure")

type errStore struct{}

func (e *errStore) Save(_ context.Context, _ domain.Collection) error  { return errStoreFail }
func (e *errStore) Delete(_ context.Context, _ domain.Namespace) error { return errStoreFail }
func (e *errStore) Get(_ context.Context, _ domain.Namespace) (*domain.Collection, error) {
	return nil, errStoreFail
}

func (e *errStore) List(_ context.Context) ([]domain.Collection, error) {
	return nil, errStoreFail
}
func (e *errStore) Close() error { return errStoreFail }

// --- registerProjections error paths ---

func TestRegisterProjections_FirstSubscribeFails_ReturnsError(t *testing.T) {
	stubErr := errors.New("subscribe error")
	ax := &failingAxQuiver{subscribeCallN: 1, err: stubErr}
	svc := &collectionService{axCollection: ax, store: &errStore{}}

	err := svc.registerProjections()
	require.Error(t, err)
	assert.ErrorIs(t, err, stubErr)
}

func TestNew_OnForgetRegistrationFails_ReturnsError(t *testing.T) {
	wantErr := assert.AnError
	ax := &failingAxQuiver{onForgetErr: wantErr}

	s, err := store.New(":memory:")
	require.NoError(t, err)

	_, err = New(ax, s, &mocks.Vault{}, &mocks.Manifold{})
	require.ErrorIs(t, err, wantErr)
}

// ─── NewFromDBPath ────────────────────────────────────────────────────────────

func TestNewFromDBPath_InvalidPath_ReturnsError(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	axCollection, err := newAsynxCollection(es, ss)
	require.NoError(t, err)
	t.Cleanup(func() { _ = axCollection.Shutdown(context.Background()) })

	_, err = NewFromDBPath(axCollection, "/nonexistent/path/to/quiver.db", &mocks.Vault{}, &mocks.Manifold{})
	require.Error(t, err)
}

func TestNewFromDBPath_Success_ReturnsNonNil(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	axCollection, err := newAsynxCollection(es, ss)
	require.NoError(t, err)
	t.Cleanup(func() { _ = axCollection.Shutdown(context.Background()) })

	repo, err := NewFromDBPath(axCollection, ":memory:", &mocks.Vault{}, &mocks.Manifold{})
	require.NoError(t, err)
	require.NotNil(t, repo)
}

// ─── Shutdown ─────────────────────────────────────────────────────────────────

func TestShutdown_DrainsAsynxCollection(t *testing.T) {
	_, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, repo.Shutdown(context.Background()))

	err := repo.Follow(context.Background(), "github.com/org/repo", &domain.Collection{}, nil)
	assert.Error(t, err, "a drained aggregate must reject new commands")
}

// closeProbeStore records what the aggregate answers at the moment the store is
// closed, so ordering is proven by a returned value rather than by timing.
type closeProbeStore struct {
	inner    store.QuiverStore
	probe    func() error
	probeErr error
	closed   bool
}

func (s *closeProbeStore) Save(ctx context.Context, coll domain.Collection) error {
	return s.inner.Save(ctx, coll)
}

func (s *closeProbeStore) Delete(ctx context.Context, ns domain.Namespace) error {
	return s.inner.Delete(ctx, ns)
}

func (s *closeProbeStore) Get(ctx context.Context, ns domain.Namespace) (*domain.Collection, error) {
	return s.inner.Get(ctx, ns)
}

func (s *closeProbeStore) List(ctx context.Context) ([]domain.Collection, error) {
	return s.inner.List(ctx)
}

func (s *closeProbeStore) Close() error {
	s.closed = true
	s.probeErr = s.probe()
	return s.inner.Close()
}

func TestShutdown_ClosesStoreOnlyAfterAggregateDrained(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	axCollection, err := newAsynxCollection(es, ss)
	require.NoError(t, err)

	inner, err := store.New(":memory:")
	require.NoError(t, err)
	probe := &closeProbeStore{inner: inner}

	svc := &collectionService{
		axCollection: axCollection,
		store:        probe,
		vault:        &mocks.Vault{},
		manifold:     &mocks.Manifold{},
	}
	probe.probe = func() error {
		return svc.Follow(context.Background(), "github.com/org/drained", makeTestManifest("drained"), nil)
	}
	require.NoError(t, svc.registerProjections())

	require.NoError(t, svc.Follow(context.Background(), "github.com/org/live", makeTestManifest("live"), nil),
		"the same command must succeed while the aggregate is live")

	require.NoError(t, svc.Shutdown(context.Background()))

	assert.True(t, probe.closed, "the collections read model must be closed on shutdown")
	assert.Error(t, probe.probeErr, "the aggregate must already be drained when the store closes")
}

func TestShutdown_StoreCloseFails_ReturnsError(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	axCollection, err := newAsynxCollection(es, ss)
	require.NoError(t, err)

	svc := &collectionService{
		axCollection: axCollection,
		store:        &errStore{},
		vault:        &mocks.Vault{},
		manifold:     &mocks.Manifold{},
	}

	err = svc.Shutdown(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, errStoreFail)
}

func TestShutdown_DrainAndCloseFail_ReturnsBothErrors(t *testing.T) {
	drainErr := errors.New("drain failed")
	svc := &collectionService{
		axCollection: &failingAxQuiver{shutdownErr: drainErr},
		store:        &errStore{},
		vault:        &mocks.Vault{},
		manifold:     &mocks.Manifold{},
	}

	err := svc.Shutdown(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, drainErr)
	assert.ErrorIs(t, err, errStoreFail)
}
