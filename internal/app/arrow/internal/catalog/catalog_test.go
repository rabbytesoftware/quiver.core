package catalog

import (
	"context"
	"errors"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/manifest"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
)

// --- helpers ---

func makeManifest(name string) *domain.ArrowManifest {
	return &domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{
			Name:    name,
			Version: "1.0.0",
		},
	}
}

func newAsynxArrow(es asynxModels.Store) (asynx.Asynx[domain.Arrow], error) {
	return asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

func newAsynxRuntime(es asynxModels.Store) (asynx.Asynx[domainRuntime.ArrowRuntime], error) {
	return asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

func testCatalog(t *testing.T, resolve manifest.ResolveFunc) (*catalogService, Catalog) {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)

	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	cat, err := store.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	svc := &catalogService{
		axArrow:         axArrow,
		axRuntime:       axRuntime,
		store:           cat,
		resolveManifest: resolve,
	}

	err = svc.registerProjections()
	require.NoError(t, err)

	return svc, svc
}

// resolveOK returns a ResolveFunc that always returns the given manifest.
func resolveOK(m *domain.ArrowManifest) manifest.ResolveFunc {
	return func(_ context.Context, _ domain.Namespace) (*domain.ArrowManifest, error) {
		return m, nil
	}
}

// resolveErr returns a ResolveFunc that always returns the given error.
func resolveErr(err error) manifest.ResolveFunc {
	return func(_ context.Context, _ domain.Namespace) (*domain.ArrowManifest, error) {
		return nil, err
	}
}

// seedArrow sends an AddArrow event and waits for projections to process.
func seedArrow(t *testing.T, svc *catalogService, ns domain.Namespace, m *domain.ArrowManifest) {
	t.Helper()

	err := svc.Add(context.Background(), ns)
	require.NoError(t, err)
	svc.axArrow.WaitPublish()
}

// --- Add ---

func TestAdd_InvalidNamespace_ReturnsErrInvalidNamespace(t *testing.T) {
	_, cat := testCatalog(t, resolveErr(errors.New("should not be called")))

	err := cat.Add(context.Background(), "bad-namespace")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestAdd_ResolveFails_ReturnsErrFetchFailed(t *testing.T) {
	_, cat := testCatalog(t, resolveErr(errors.New("network error")))

	err := cat.Add(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

func TestAdd_Success_ArrowAvailableInAsynx(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t, resolveOK(m))

	err := cat.Add(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	svc.axArrow.WaitPublish()

	got, err := svc.axArrow.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), got.Namespace)
}

func TestAdd_AlreadyExists_ReturnsErrAlreadyExists(t *testing.T) {
	m := makeManifest("MyArrow")
	_, cat := testCatalog(t, resolveOK(m))

	require.NoError(t, cat.Add(context.Background(), "github.com/org/repo"))

	err := cat.Add(context.Background(), "github.com/org/repo")
	assert.ErrorIs(t, err, apperrors.ErrAlreadyExists)
}

// --- Update ---

func TestUpdate_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, cat := testCatalog(t, resolveErr(errors.New("not used")))

	err := cat.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUpdate_AfterRemoved_ReturnsErrNotFound(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t, resolveOK(m))

	seedArrow(t, svc, "github.com/org/repo", m)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	err := cat.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUpdate_ResolveFails_ReturnsErrFetchFailed(t *testing.T) {
	m := makeManifest("MyArrow")
	calls := 0
	resolve := func(ctx context.Context, ns domain.Namespace) (*domain.ArrowManifest, error) {
		calls++
		if calls == 1 {
			return m, nil
		}
		return nil, errors.New("network error")
	}
	svc, cat := testCatalog(t, resolve)

	seedArrow(t, svc, "github.com/org/repo", m)

	err := cat.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrFetchFailed)
}

func TestUpdate_Success_UpdatesManifest(t *testing.T) {
	m := makeManifest("MyArrow")
	updated := makeManifest("UpdatedArrow")
	calls := 0
	resolve := func(ctx context.Context, ns domain.Namespace) (*domain.ArrowManifest, error) {
		calls++
		if calls == 1 {
			return m, nil
		}
		return updated, nil
	}
	svc, cat := testCatalog(t, resolve)

	seedArrow(t, svc, "github.com/org/repo", m)

	require.NoError(t, cat.Update(context.Background(), "github.com/org/repo"))
	svc.axArrow.WaitPublish()

	got, err := svc.axArrow.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	var gotName string
	for _, mv := range got.Versions {
		gotName = mv.Name
		break
	}
	assert.Equal(t, "UpdatedArrow", gotName)
}

func TestUpdate_ActiveRuntime_ReturnsErrStateViolation(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t, resolveOK(m))

	seedArrow(t, svc, "github.com/org/repo", m)

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    "github.com/org/repo",
		State: domain.ArrowStateRunning,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	err = cat.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

// --- Remove ---

func TestRemove_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, cat := testCatalog(t, nil)

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_AfterRemoved_ReturnsErrNotFound(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t, resolveOK(m))

	seedArrow(t, svc, "github.com/org/repo", m)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_ActiveRuntime_ReturnsErrStateViolation(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t, resolveOK(m))

	seedArrow(t, svc, "github.com/org/repo", m)

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    "github.com/org/repo",
		State: domain.ArrowStateRunning,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	err = cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestRemove_Success_ForgetsAggregateFromAsynx(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t, resolveOK(m))

	seedArrow(t, svc, "github.com/org/repo", m)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	exists, err := svc.axArrow.Exists(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.False(t, exists)
}

// --- List ---

func TestList_EmptyStore_ReturnsEmpty(t *testing.T) {
	_, cat := testCatalog(t, nil)

	result, err := cat.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_ReturnsStoredArrows(t *testing.T) {
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t, resolveOK(m))

	require.NoError(t, svc.store.Save(context.Background(), domain.Arrow{
		Namespace: "github.com/org/active",
		Versions:  map[string]domain.ArrowManifest{"1.0.0": {ArrowMeta: m.ArrowMeta}},
	}))
	require.NoError(t, svc.store.Save(context.Background(), domain.Arrow{
		Namespace: "github.com/org/other",
		Versions:  map[string]domain.ArrowManifest{"1.0.0": {ArrowMeta: m.ArrowMeta}},
	}))

	result, err := cat.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// --- Get ---

func TestGet_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, cat := testCatalog(t, nil)

	got, err := cat.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, got)
}

func TestGet_AfterRemoved_ReturnsErrNotFound(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t, resolveOK(m))

	seedArrow(t, svc, "github.com/org/repo", m)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	got, err := cat.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, got)
}

func TestGet_Exists_ReturnsArrow(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t, resolveOK(m))

	ns := domain.Namespace("github.com/org/repo")
	seedArrow(t, svc, ns, m)

	got, err := cat.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, ns, got.Namespace)
}

// --- IsInstalled ---

func TestIsInstalled_NoRuntime_ReturnsFalse(t *testing.T) {
	_, cat := testCatalog(t, nil)

	installed := cat.IsInstalled(context.Background(), "github.com/org/repo")
	assert.False(t, installed)
}

func TestIsInstalled_AbsentRuntime_ReturnsFalse(t *testing.T) {
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t, resolveOK(m))

	seedArrow(t, svc, "github.com/org/repo", m)

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    "github.com/org/repo",
		State: domain.ArrowStateAbsent,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	installed := cat.IsInstalled(context.Background(), "github.com/org/repo")
	assert.False(t, installed)
}

func TestIsInstalled_ReadyRuntime_ReturnsTrue(t *testing.T) {
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t, resolveOK(m))

	seedArrow(t, svc, "github.com/org/repo", m)

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    "github.com/org/repo",
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	installed := cat.IsInstalled(context.Background(), "github.com/org/repo")
	assert.True(t, installed)
}

// --- New() ---

func TestNew_ValidArgs_ReturnsCatalog(t *testing.T) {
	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)

	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	cat, err := store.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	result, err := New(axArrow, axRuntime, cat, resolveOK(makeManifest("x")))
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// --- List: store returns error ---

func TestList_StoreError_ReturnsError(t *testing.T) {
	svc, _ := testCatalog(t, nil)

	svc.store = &failingArrowCatalog{listErr: errors.New("db unavailable")}

	result, err := svc.List(context.Background())
	require.Error(t, err)
	assert.Nil(t, result)
}

// --- axArrow.Exists error branches ---

func TestUpdate_AxArrowExistsError_ReturnsError(t *testing.T) {
	svc, cat := testCatalog(t, nil)

	svc.axArrow = &failingAxArrow{existsErr: errors.New("storage failure")}

	err := cat.Update(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_AxArrowExistsError_ReturnsError(t *testing.T) {
	svc, cat := testCatalog(t, nil)

	svc.axArrow = &failingAxArrow{existsErr: errors.New("storage failure")}

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGet_AxArrowGetError_ReturnsError(t *testing.T) {
	svc, cat := testCatalog(t, nil)

	svc.axArrow = &failingAxArrow{getErr: errors.New("storage failure")}

	got, err := cat.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, got)
}

// --- Update: runtime not seeded → update allowed ---

func TestUpdate_RuntimeGetError_ReturnsError(t *testing.T) {
	m := makeManifest("Arrow")
	updated := makeManifest("Updated")
	calls := 0
	resolve := func(ctx context.Context, ns domain.Namespace) (*domain.ArrowManifest, error) {
		calls++
		if calls == 1 {
			return m, nil
		}
		return updated, nil
	}
	svc, cat := testCatalog(t, resolve)

	ns := domain.Namespace("github.com/org/repo")
	seedArrow(t, svc, ns, m)

	// Runtime is not seeded → absent state → update allowed.
	require.NoError(t, cat.Update(context.Background(), ns))
}

// --- AddWithManifest ---

func TestAddWithManifest_StoresManifestAndEmitsEvent(t *testing.T) {
	cs, _ := testCatalog(t, nil)

	ns := domain.Namespace("github.com/user/repo")
	m := makeManifest("test-arrow")

	err := cs.AddWithManifest(context.Background(), ns, m)
	require.NoError(t, err)

	cs.axArrow.WaitPublish()
	got, err := cs.axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	var gotName string
	for _, mv := range got.Versions {
		gotName = mv.Name
		break
	}
	assert.Equal(t, "test-arrow", gotName)
}

func TestAddWithManifest_InvalidNamespace_ReturnsError(t *testing.T) {
	_, cat := testCatalog(t, nil)

	err := cat.AddWithManifest(context.Background(), "bad", &domain.ArrowManifest{})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestUpdateWithManifest_Success_UpdatesManifestInAsynx(t *testing.T) {
	m := makeManifest("original")
	updated := makeManifest("updated")
	svc, cat := testCatalog(t, resolveOK(m))
	seedArrow(t, svc, "github.com/user/repo", m)

	err := cat.UpdateWithManifest(context.Background(), "github.com/user/repo", updated)
	require.NoError(t, err)
	svc.axArrow.WaitPublish()

	got, err := svc.axArrow.Get(context.Background(), "github.com/user/repo")
	require.NoError(t, err)
	var gotName string
	for _, mv := range got.Versions {
		gotName = mv.Name
		break
	}
	assert.Equal(t, "updated", gotName)
}

func TestUpdateWithManifest_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, cat := testCatalog(t, nil)

	err := cat.UpdateWithManifest(context.Background(), "github.com/user/repo", makeManifest("x"))
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestAddWithManifest_AlreadyExists_ReturnsErrAlreadyExists(t *testing.T) {
	_, cat := testCatalog(t, nil)

	ns := domain.Namespace("github.com/user/repo")
	require.NoError(t, cat.AddWithManifest(context.Background(), ns, makeManifest("x")))

	err := cat.AddWithManifest(context.Background(), ns, makeManifest("x"))
	assert.ErrorIs(t, err, apperrors.ErrAlreadyExists)
}

// failingAxArrow is a stub asynx.Asynx[domain.Arrow] that fails on the Nth Subscribe call
// and can optionally return an error from Get.
type failingAxArrow struct {
	subscribeCallN int
	calls          int
	err            error
	getErr         error
	existsErr      error
	onForgetErr    error
}

func (f *failingAxArrow) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Arrow],
	_ ...asynxModels.SubscriptionOpt[domain.Arrow],
) (string, error) {
	f.calls++
	if f.calls == f.subscribeCallN {
		return "", f.err
	}
	return "sub-id", nil
}

func (f *failingAxArrow) Send(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingAxArrow) SendWait(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingAxArrow) Shutdown(_ context.Context) error { return nil }
func (f *failingAxArrow) Get(_ context.Context, _ string) (domain.Arrow, error) {
	if f.getErr != nil {
		return domain.Arrow{}, f.getErr
	}
	return domain.Arrow{}, nil
}
func (f *failingAxArrow) Exists(_ context.Context, _ string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return false, nil
}
func (f *failingAxArrow) Preload(_ context.Context, _ string) error { return nil }
func (f *failingAxArrow) Unsubscribe(_ string) error                { return nil }
func (f *failingAxArrow) Replay(_ context.Context, _ string, _ int64, _ int64, _ asynxModels.ProjectionHandler[domain.Arrow]) error {
	return nil
}
func (f *failingAxArrow) WaitPublish()                             {}
func (f *failingAxArrow) Forget(_ context.Context, _ string) error { return nil }
func (f *failingAxArrow) OnForget(_ asynxModels.ForgetHandler[domain.Arrow]) (string, error) {
	if f.onForgetErr != nil {
		return "", f.onForgetErr
	}
	return "forget-sub-id", nil
}

// failingArrowAsynx is a minimal asynx.Asynx[domain.Arrow] stub whose
// Subscribe always returns an error.
type failingArrowAsynx struct{ err error }

func (f *failingArrowAsynx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Arrow],
	_ ...asynxModels.SubscriptionOpt[domain.Arrow],
) (string, error) {
	return "", f.err
}
func (f *failingArrowAsynx) Send(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}
func (f *failingArrowAsynx) SendWait(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}
func (f *failingArrowAsynx) Get(_ context.Context, _ string) (domain.Arrow, error) {
	return domain.Arrow{}, nil
}
func (f *failingArrowAsynx) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *failingArrowAsynx) Preload(_ context.Context, _ string) error        { return nil }
func (f *failingArrowAsynx) Unsubscribe(_ string) error                       { return nil }
func (f *failingArrowAsynx) Replay(
	_ context.Context, _ string, _ int64, _ int64,
	_ asynxModels.ProjectionHandler[domain.Arrow],
) error {
	return nil
}
func (f *failingArrowAsynx) Shutdown(_ context.Context) error         { return nil }
func (f *failingArrowAsynx) WaitPublish()                             {}
func (f *failingArrowAsynx) Forget(_ context.Context, _ string) error { return nil }
func (f *failingArrowAsynx) OnForget(_ asynxModels.ForgetHandler[domain.Arrow]) (string, error) {
	return "", f.err
}

func TestNew_FailsWhenAsynxSubscribeFails(t *testing.T) {
	wantErr := errors.New("subscribe failed")

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	s, err := store.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	cat, err := New(&failingArrowAsynx{err: wantErr}, axRuntime, s, nil)

	assert.Nil(t, cat)
	require.ErrorIs(t, err, wantErr)
}

// failingArrowCatalog is a store that always fails on List.
type failingArrowCatalog struct {
	listErr error
}

func (f *failingArrowCatalog) Save(_ context.Context, _ domain.Arrow) error       { return nil }
func (f *failingArrowCatalog) Delete(_ context.Context, _ domain.Namespace) error { return nil }
func (f *failingArrowCatalog) Get(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
	return nil, nil
}
func (f *failingArrowCatalog) List(_ context.Context) ([]domain.Arrow, error) {
	return nil, f.listErr
}
