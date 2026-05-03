package quiver

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/quiver/internal/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
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

func testRepository(
	t *testing.T,
	v vault.Vault,
	m *mocks.Manifold,
) (*quiverService, Quiver) {
	t.Helper()

	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	s, err := store.New(":memory:")
	require.NoError(t, err)

	svc := &quiverService{
		axQuiver: axQuiver,
		store:    s,
		vault:    v,
		manifold: m,
	}

	require.NoError(t, svc.registerProjections())

	return svc, svc
}

func seedQuiver(
	t *testing.T,
	svc *quiverService,
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

	require.NoError(t, svc.Add(context.Background(), ns))
	svc.axQuiver.WaitPublish()
}

// --- Add ---

func TestAdd_InvalidNamespace_ReturnsErrInvalidNamespace(t *testing.T) {
	_, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	err := repo.Add(context.Background(), "bad-namespace")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestAdd_ManifoldFails_ReturnsErrFetchFailed(t *testing.T) {
	mv := &mocks.Vault{GetQuiverErr: vault.ErrNotCached}
	mm := &mocks.Manifold{ResolveQuiverErr: errors.New("network error")}
	_, repo := testRepository(t, mv, mm)

	err := repo.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

func TestAdd_Success_QuiverSentToAsynx(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, repo := testRepository(t, mv, mm)

	require.NoError(t, repo.Add(context.Background(), "github.com/org/repo"))
	svc.axQuiver.WaitPublish()

	got, err := svc.axQuiver.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), got.Namespace)
}

func TestAdd_AlreadyExists_ReturnsError(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, repo := testRepository(t, mv, mm)

	require.NoError(t, repo.Add(context.Background(), "github.com/org/repo"))
	svc.axQuiver.WaitPublish()

	err := repo.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
}

// --- Remove ---

func TestRemove_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	err := repo.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_Success_ForgetsAggregateFromAsynx(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, repo := testRepository(t, mv, mm)

	ns := domain.Namespace("github.com/org/repo")
	seedQuiver(t, svc, ns, manifest, mv, mm)

	require.NoError(t, repo.Remove(context.Background(), ns))

	exists, err := svc.axQuiver.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRemove_AfterRemoved_ReturnsErrNotFound(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, repo := testRepository(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	require.NoError(t, repo.Remove(context.Background(), "github.com/org/repo"))

	err := repo.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
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

	require.NoError(t, svc.store.Save(context.Background(), domain.Quiver{
		Namespace: "github.com/org/one",
	}))
	require.NoError(t, svc.store.Save(context.Background(), domain.Quiver{
		Namespace: "github.com/org/two",
	}))

	result, err := repo.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// --- Get ---

func TestGet_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	detail, err := repo.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, detail)
}

func TestGet_Found_ReturnsQuiver(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	require.NoError(t, svc.store.Save(context.Background(), domain.Quiver{
		Namespace: ns,
	}))

	detail, err := repo.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, ns, detail.Namespace)
}

// --- OnQuiverAdded ---

func TestOnQuiverAdded_CallbackFires_OnAdd(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, repo := testRepository(t, mv, mm)

	var fired atomic.Bool
	require.NoError(t, repo.OnQuiverAdded(func(_ context.Context, q domain.Quiver) {
		fired.Store(true)
	}))

	require.NoError(t, repo.Add(context.Background(), "github.com/org/repo"))
	svc.axQuiver.WaitPublish()

	assert.True(t, fired.Load())
}

// --- OnQuiverUpdated ---

func TestOnQuiverUpdated_CallbackFires_OnUpdate(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, repo := testRepository(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	var fired atomic.Bool
	require.NoError(t, repo.OnQuiverUpdated(func(_ context.Context, q domain.Quiver) {
		fired.Store(true)
	}))

	mm.ResolveQuiverManifest = makeTestManifest("UpdatedQuiver")
	require.NoError(t, repo.Update(context.Background(), "github.com/org/repo"))
	svc.axQuiver.WaitPublish()

	assert.True(t, fired.Load())
}

// --- OnQuiverRemoved ---

func TestOnQuiverRemoved_CallbackFires_OnRemove(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, repo := testRepository(t, mv, mm)

	ns := domain.Namespace("github.com/org/repo")
	seedQuiver(t, svc, ns, manifest, mv, mm)

	var captured domain.Namespace
	require.NoError(t, repo.OnQuiverRemoved(func(_ context.Context, removed domain.Namespace) {
		captured = removed
	}))

	require.NoError(t, repo.Remove(context.Background(), ns))
	svc.axQuiver.WaitPublish()

	assert.Equal(t, ns, captured)
}

// --- New constructor ---

func TestNew_Success_ReturnsNonNilRepository(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	s, err := store.New(":memory:")
	require.NoError(t, err)

	repo, err := New(axQuiver, s, &mocks.Vault{}, &mocks.Manifold{})
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
	getResult      domain.Quiver
	existsErr      error
	existsResult   bool
	sendErr        error
	forgetErr      error
	onForgetErr    error
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

func (f *failingAxQuiver) Shutdown(_ context.Context) error { return nil }
func (f *failingAxQuiver) Get(_ context.Context, _ string) (domain.Quiver, error) {
	return f.getResult, f.getErr
}

func (f *failingAxQuiver) Exists(_ context.Context, _ string) (bool, error) {
	return f.existsResult, f.existsErr
}
func (f *failingAxQuiver) Preload(_ context.Context, _ string) error { return nil }
func (f *failingAxQuiver) Unsubscribe(_ string) error                { return nil }
func (f *failingAxQuiver) Replay(_ context.Context, _ string, _, _ int64, _ asynxModels.ProjectionHandler[domain.Quiver]) error {
	return nil
}
func (f *failingAxQuiver) WaitPublish()                             {}
func (f *failingAxQuiver) Forget(_ context.Context, _ string) error { return f.forgetErr }
func (f *failingAxQuiver) OnForget(_ asynxModels.ForgetHandler[domain.Quiver]) (string, error) {
	if f.onForgetErr != nil {
		return "", f.onForgetErr
	}
	return "forget-sub-id", nil
}

func (f *failingAxQuiver) Listen(_ string, _ int) (<-chan asynxModels.Event[domain.Quiver], func(), error) {
	return nil, func() {}, nil
}

func (f *failingAxQuiver) SubscribeWait(_ context.Context, _ string) (asynxModels.Event[domain.Quiver], error) {
	return asynxModels.Event[domain.Quiver]{}, nil
}

// --- errStore ---

var errStoreFail = errors.New("store failure")

type errStore struct{}

func (e *errStore) Save(_ context.Context, _ domain.Quiver) error      { return errStoreFail }
func (e *errStore) Delete(_ context.Context, _ domain.Namespace) error { return errStoreFail }
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
	svc := &quiverService{axQuiver: ax, store: &errStore{}}

	err := svc.registerProjections()
	require.Error(t, err)
	assert.ErrorIs(t, err, stubErr)
}

func TestRegisterProjections_SecondSubscribeFails_ReturnsError(t *testing.T) {
	stubErr := errors.New("subscribe error 2")
	ax := &failingAxQuiver{subscribeCallN: 2, err: stubErr}
	svc := &quiverService{axQuiver: ax, store: &errStore{}}

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

// --- Update error paths ---

func TestUpdate_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	err := repo.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUpdate_ManifoldFails_ReturnsErrFetchFailed(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, repo := testRepository(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	mm.ResolveQuiverErr = errors.New("network error")
	mm.ResolveQuiverManifest = nil

	err := repo.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

func TestUpdate_VaultPutFails_ReturnsError(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, repo := testRepository(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	mv.PutQuiverErr = errors.New("vault full")
	mv.PutQuiverPath = ""
	mm.ResolveQuiverManifest = makeTestManifest("Updated")
	mm.ResolveQuiverErr = nil

	err := repo.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
}

func TestUpdate_ExistsUnexpectedError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("unexpected exists error")
	ax := &failingAxQuiver{existsErr: unexpectedErr}
	svc := &quiverService{
		axQuiver: ax,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
	}

	err := svc.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, unexpectedErr)
}

// --- Remove error paths ---

func TestRemove_ExistsUnexpectedError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("unexpected exists error")
	ax := &failingAxQuiver{existsErr: unexpectedErr}
	svc := &quiverService{
		axQuiver: ax,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
	}

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, unexpectedErr)
}

func TestRemove_ForgetFails_ReturnsError(t *testing.T) {
	forgetErr := errors.New("forget error")
	ax := &failingAxQuiver{
		existsResult: true,
		forgetErr:    forgetErr,
	}
	svc := &quiverService{
		axQuiver: ax,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
	}

	err := svc.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, forgetErr)
}

// --- List error path ---

func TestList_StoreError_ReturnsError(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	svc := &quiverService{
		axQuiver: axQuiver,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
	}

	_, err = svc.List(context.Background())
	require.Error(t, err)
}

// --- Get error path ---

func TestGet_StoreError_ReturnsError(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)

	svc := &quiverService{
		axQuiver: axQuiver,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
	}

	_, err = svc.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
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
	svc, _ := testRepository(t, mv, mm)

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
	svc, _ := testRepository(t, mv, mm)

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
	svc, _ := testRepository(t, mv, mm)

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
	svc, _ := testRepository(t, mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, freshManifest, got)
	assert.Equal(t, "/home/new", path)
	assert.Equal(t, 1, mv.PutQuiverCalls)
}

func TestResolveManifest_NotCachedManifoldFails_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{GetQuiverErr: vault.ErrNotCached}
	mm := &mocks.Manifold{ResolveQuiverErr: errors.New("network error")}
	svc, _ := testRepository(t, mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

func TestResolveManifest_UnexpectedVaultError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("disk failure")
	mv := &mocks.Vault{GetQuiverErr: unexpectedErr}
	mm := &mocks.Manifold{}
	svc, _ := testRepository(t, mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.ErrorIs(t, err, unexpectedErr)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

// ─── Add: Send generic error ──────────────────────────────────────────────────

func TestAdd_SendGenericError_ReturnsError(t *testing.T) {
	sendErr := errors.New("send error")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: makeTestManifest("test")}
	ax := &failingAxQuiver{sendErr: sendErr}
	svc := &quiverService{axQuiver: ax, store: &errStore{}, vault: mv, manifold: mm}

	err := svc.Add(context.Background(), "github.com/org/repo@v1.0.0")
	require.Error(t, err)
	assert.ErrorIs(t, err, sendErr)
}

// ─── Update: Send error paths ─────────────────────────────────────────────────

func TestUpdate_SendErrNotFound_ReturnsErrNotFound(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, _ := testRepository(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	ax := &failingAxQuiver{existsResult: true, sendErr: asynxModels.ErrNotFound}
	svc2 := &quiverService{axQuiver: ax, store: svc.store, vault: mv, manifold: mm}
	mm.ResolveQuiverManifest = makeTestManifest("Updated")
	mm.ResolveQuiverErr = nil
	mv.PutQuiverErr = nil
	mv.PutQuiverPath = "/tmp/updated"

	err := svc2.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUpdate_SendGenericError_ReturnsError(t *testing.T) {
	manifest := makeTestManifest("MyQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/tmp/test",
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: manifest}
	svc, _ := testRepository(t, mv, mm)

	seedQuiver(t, svc, "github.com/org/repo", manifest, mv, mm)

	sendErr := errors.New("generic send error")
	ax := &failingAxQuiver{existsResult: true, sendErr: sendErr}
	svc2 := &quiverService{axQuiver: ax, store: svc.store, vault: mv, manifold: mm}
	mm.ResolveQuiverManifest = makeTestManifest("Updated")
	mm.ResolveQuiverErr = nil
	mv.PutQuiverErr = nil
	mv.PutQuiverPath = "/tmp/updated"

	err := svc2.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, sendErr)
}

// ─── resolveManifest: vault put error paths ───────────────────────────────────

func TestResolveManifest_StaleVault_PutFails_ReturnsError(t *testing.T) {
	staleManifest := makeTestManifest("StaleQuiver")
	freshManifest := makeTestManifest("FreshQuiver")
	putErr := errors.New("put error")
	mv := &mocks.Vault{
		GetQuiverEntry: &vault.QuiverVaultEntry{Manifest: staleManifest},
		GetQuiverPath:  "/home/stale",
		GetQuiverErr:   vault.ErrStale,
		PutQuiverErr:   putErr,
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: freshManifest}
	svc, _ := testRepository(t, mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, putErr)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

func TestResolveManifest_NotCached_PutFails_ReturnsError(t *testing.T) {
	freshManifest := makeTestManifest("FreshQuiver")
	putErr := errors.New("put error")
	mv := &mocks.Vault{
		GetQuiverErr: vault.ErrNotCached,
		PutQuiverErr: putErr,
	}
	mm := &mocks.Manifold{ResolveQuiverManifest: freshManifest}
	svc, _ := testRepository(t, mv, mm)

	got, path, err := svc.resolveManifest(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, putErr)
	assert.Nil(t, got)
	assert.Empty(t, path)
}

// ─── NewFromDBPath ────────────────────────────────────────────────────────────

func TestNewFromDBPath_InvalidPath_ReturnsError(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)
	t.Cleanup(func() { _ = axQuiver.Shutdown(context.Background()) })

	_, err = NewFromDBPath(axQuiver, "/nonexistent/path/to/quiver.db", &mocks.Vault{}, &mocks.Manifold{})
	require.Error(t, err)
}

func TestNewFromDBPath_Success_ReturnsNonNil(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axQuiver, err := newAsynxQuiver(es)
	require.NoError(t, err)
	t.Cleanup(func() { _ = axQuiver.Shutdown(context.Background()) })

	repo, err := NewFromDBPath(axQuiver, ":memory:", &mocks.Vault{}, &mocks.Manifold{})
	require.NoError(t, err)
	require.NotNil(t, repo)
}
