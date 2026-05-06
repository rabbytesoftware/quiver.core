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

func makeTestManifest(name string) *domain.Quiver {
	return &domain.Quiver{
		Meta: domain.QuiverMeta{
			Name:        name,
			Description: "A test quiver",
			Tags:        []string{"test"},
		},
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
) {
	t.Helper()
	require.NoError(t, svc.Follow(context.Background(), ns, &domain.Quiver{}, nil))
	svc.axQuiver.WaitPublish()
}

// --- Follow ---

func TestFollow_InvalidNamespace_ReturnsErrInvalidNamespace(t *testing.T) {
	_, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	err := repo.Follow(context.Background(), "bad-namespace", &domain.Quiver{}, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestFollow_ErrorsOnDuplicate(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, repo.Follow(context.Background(), "github.com/org/repo", &domain.Quiver{}, nil))
	svc.axQuiver.WaitPublish()

	err := repo.Follow(context.Background(), "github.com/org/repo", &domain.Quiver{}, nil)
	require.Error(t, err)
}

func TestFollow_StoresFollowState(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	require.NoError(t, repo.Follow(context.Background(), "github.com/org/repo", &domain.Quiver{}, nil))
	svc.axQuiver.WaitPublish()

	got, err := svc.axQuiver.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), got.Namespace)
}

func TestFollow_SendGenericError_ReturnsError(t *testing.T) {
	sendErr := errors.New("send error")
	ax := &failingAxQuiver{sendErr: sendErr}
	svc := &quiverService{axQuiver: ax, store: &errStore{}, vault: &mocks.Vault{}, manifold: &mocks.Manifold{}}

	err := svc.Follow(context.Background(), "github.com/org/repo@v1.0.0", &domain.Quiver{}, nil)
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
	seedQuiver(t, svc, ns)

	require.NoError(t, repo.Unfollow(context.Background(), ns))

	exists, err := svc.axQuiver.Exists(context.Background(), ns.String())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestUnfollow_AfterUnfollowed_ReturnsErrNotFound(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	seedQuiver(t, svc, "github.com/org/repo")

	require.NoError(t, repo.Unfollow(context.Background(), "github.com/org/repo"))

	err := repo.Unfollow(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUnfollow_ExistsUnexpectedError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("unexpected exists error")
	ax := &failingAxQuiver{existsErr: unexpectedErr}
	svc := &quiverService{
		axQuiver: ax,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
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
	svc := &quiverService{
		axQuiver: ax,
		store:    &errStore{},
		vault:    &mocks.Vault{},
		manifold: &mocks.Manifold{},
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

// --- Get (vault cache-or-fetch) ---

func TestGet_FreshVaultHit_ReturnsManifest(t *testing.T) {
	manifest := makeTestManifest("FreshQuiver")
	mv := &mocks.Vault{
		GetQuiverEntry: &vault.QuiverVaultEntry{Quiver: manifest},
		GetQuiverPath:  "/home/fresh",
		GetQuiverErr:   nil,
	}
	mm := &mocks.Manifold{}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, manifest, got)

	assert.Equal(t, 0, mv.PutQuiverCalls)
}

func TestGet_StaleVaultManifoldSucceeds_ReturnsFreshManifest(t *testing.T) {
	staleManifest := makeTestManifest("StaleQuiver")
	freshManifest := makeTestManifest("FreshQuiver")
	mv := &mocks.Vault{
		GetQuiverEntry: &vault.QuiverVaultEntry{Quiver: staleManifest},
		GetQuiverPath:  "/home/stale",
		GetQuiverErr:   vault.ErrStale,
		PutQuiverPath:  "/home/fresh",
	}
	mm := &mocks.Manifold{ResolveQuiverResult: freshManifest}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, freshManifest, got)

	assert.Equal(t, 1, mv.PutQuiverCalls)
}

func TestGet_StaleVaultManifoldFails_ReturnsStaleManifest(t *testing.T) {
	staleManifest := makeTestManifest("StaleQuiver")
	mv := &mocks.Vault{
		GetQuiverEntry: &vault.QuiverVaultEntry{Quiver: staleManifest},
		GetQuiverPath:  "/home/stale",
		GetQuiverErr:   vault.ErrStale,
	}
	mm := &mocks.Manifold{ResolveQuiverErr: errors.New("network error")}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, staleManifest, got)

	assert.Equal(t, 0, mv.PutQuiverCalls)
}

func TestGet_NotCachedManifoldSucceeds_ReturnsManifestAndStores(t *testing.T) {
	freshManifest := makeTestManifest("NewQuiver")
	mv := &mocks.Vault{
		GetQuiverErr:  vault.ErrNotCached,
		PutQuiverPath: "/home/new",
	}
	mm := &mocks.Manifold{ResolveQuiverResult: freshManifest}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, freshManifest, got)

	assert.Equal(t, 1, mv.PutQuiverCalls)
}

func TestGet_NotCachedManifoldFails_ReturnsError(t *testing.T) {
	mv := &mocks.Vault{GetQuiverErr: vault.ErrNotCached}
	mm := &mocks.Manifold{ResolveQuiverErr: errors.New("network error")}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestGet_UnexpectedVaultError_ReturnsError(t *testing.T) {
	unexpectedErr := errors.New("disk failure")
	mv := &mocks.Vault{GetQuiverErr: unexpectedErr}
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
		GetQuiverEntry: &vault.QuiverVaultEntry{Quiver: staleManifest},
		GetQuiverPath:  "/home/stale",
		GetQuiverErr:   vault.ErrStale,
		PutQuiverErr:   putErr,
	}
	mm := &mocks.Manifold{ResolveQuiverResult: freshManifest}
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
		GetQuiverErr: vault.ErrNotCached,
		PutQuiverErr: putErr,
	}
	mm := &mocks.Manifold{ResolveQuiverResult: freshManifest}
	_, repo := testRepository(t, mv, mm)

	got, err := repo.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, putErr)
	assert.Nil(t, got)
}

// --- IsFollowed ---

func TestIsFollowed_WhenFollowed_ReturnsTrue(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	seedQuiver(t, svc, "github.com/org/repo")

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

// --- OnQuiverFollowed ---

func TestOnQuiverFollowed_CallbackFires_OnFollow(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	var fired atomic.Bool
	require.NoError(t, repo.OnQuiverFollowed(func(_ context.Context, q domain.Quiver) {
		fired.Store(true)
	}))

	require.NoError(t, repo.Follow(context.Background(), "github.com/org/repo", &domain.Quiver{}, nil))
	svc.axQuiver.WaitPublish()

	assert.True(t, fired.Load())
}

// --- OnQuiverUnfollowed ---

func TestOnQuiverUnfollowed_CallbackFires_OnUnfollow(t *testing.T) {
	svc, repo := testRepository(t, &mocks.Vault{}, &mocks.Manifold{})

	ns := domain.Namespace("github.com/org/repo")
	seedQuiver(t, svc, ns)

	var captured domain.Namespace
	require.NoError(t, repo.OnQuiverUnfollowed(func(_ context.Context, removed domain.Namespace) {
		captured = removed
	}))

	require.NoError(t, repo.Unfollow(context.Background(), ns))
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
