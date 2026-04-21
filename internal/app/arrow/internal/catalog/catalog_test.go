package catalog

import (
	"context"
	"errors"
	"testing"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
)

func makeManifest(
	name string,
) *domain.Arrow {
	return &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:    name,
			Version: "1.0.0",
		},
	}
}

func newAsynxArrow(
	es asynxModels.Store,
) (asynx.Asynx[domain.Arrow], error) {
	return asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

func newAsynxRuntime(
	es asynxModels.Store,
) (asynx.Asynx[domainRuntime.ArrowRuntime], error) {
	return asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
}

func testCatalog(
	t *testing.T,
) (*catalogService, Catalog) {
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
		axArrow:   axArrow,
		axRuntime: axRuntime,
		store:     cat,
	}

	err = svc.registerProjections()
	require.NoError(t, err)

	return svc, svc
}

func seedArrow(
	t *testing.T,
	svc *catalogService,
	ns domain.Namespace,
	m *domain.Arrow,
) {
	t.Helper()

	err := svc.Add(context.Background(), ns, m, true, "")
	require.NoError(t, err)
	svc.axArrow.WaitPublish()
}

func TestAdd_InvalidNamespace_ReturnsErrInvalidNamespace(t *testing.T) {
	_, cat := testCatalog(t)

	err := cat.Add(context.Background(), "bad-namespace", makeManifest("x"), true, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrInvalidNamespace)
}

func TestAdd_Success_ArrowAvailableInAsynx(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

	err := cat.Add(context.Background(), "github.com/org/repo", m, true, "")
	require.NoError(t, err)
	svc.axArrow.WaitPublish()

	got, err := svc.axArrow.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, domain.Namespace("github.com/org/repo"), got.Namespace)
}

func TestAdd_AlreadyExists_ReturnsNil(t *testing.T) {
	m := makeManifest("MyArrow")
	_, cat := testCatalog(t)

	require.NoError(t, cat.Add(context.Background(), "github.com/org/repo", m, true, ""))

	err := cat.Add(context.Background(), "github.com/org/repo", m, true, "")
	assert.NoError(t, err)
}

func TestUpdate_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, cat := testCatalog(t)

	err := cat.Update(context.Background(), "github.com/org/repo", makeManifest("x"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUpdate_AfterRemoved_ReturnsErrNotFound(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	err := cat.Update(context.Background(), "github.com/org/repo", makeManifest("x"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestUpdate_Success_UpdatesManifest(t *testing.T) {
	m := makeManifest("MyArrow")
	updated := makeManifest("UpdatedArrow")
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	require.NoError(t, cat.Update(context.Background(), "github.com/org/repo", updated))
	svc.axArrow.WaitPublish()

	got, err := svc.axArrow.Get(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.Equal(t, "UpdatedArrow", got.Name)
}

func TestUpdate_ActiveRuntime_ReturnsErrStateViolation(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	_, err := svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    "github.com/org/repo",
		State: domain.ArrowStateRunning,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	err = cat.Update(context.Background(), "github.com/org/repo", makeManifest("x"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestRemove_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, cat := testCatalog(t)

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_AfterRemoved_ReturnsErrNotFound(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_ActiveRuntime_ReturnsErrStateViolation(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

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
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	exists, err := svc.axArrow.Exists(context.Background(), "github.com/org/repo")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestList_EmptyStore_ReturnsEmpty(t *testing.T) {
	_, cat := testCatalog(t)

	result, err := cat.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestList_ReturnsStoredArrows(t *testing.T) {
	m := makeManifest("Arrow")
	svc, _ := testCatalog(t)

	require.NoError(t, svc.store.Save(context.Background(), domain.Arrow{
		Namespace: "github.com/org/active",
		ArrowMeta: m.ArrowMeta,
	}))
	require.NoError(t, svc.store.Save(context.Background(), domain.Arrow{
		Namespace: "github.com/org/other",
		ArrowMeta: m.ArrowMeta,
	}))

	result, err := svc.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestGet_NotFound_ReturnsErrNotFound(t *testing.T) {
	_, cat := testCatalog(t)

	got, err := cat.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, got)
}

func TestGet_AfterRemoved_ReturnsErrNotFound(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	require.NoError(t, cat.Remove(context.Background(), "github.com/org/repo"))

	got, err := cat.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, got)
}

func TestGet_Exists_ReturnsArrow(t *testing.T) {
	m := makeManifest("MyArrow")
	svc, cat := testCatalog(t)

	ns := domain.Namespace("github.com/org/repo")
	seedArrow(t, svc, ns, m)

	got, err := cat.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, ns, got.Namespace)
}

func TestIsInstalled_NoRuntime_ReturnsFalse(t *testing.T) {
	_, cat := testCatalog(t)

	installed := cat.IsInstalled(context.Background(), "github.com/org/repo")
	assert.False(t, installed)
}

func TestIsInstalled_AbsentRuntime_ReturnsFalse(t *testing.T) {
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t)

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
	svc, cat := testCatalog(t)

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

	result, err := New(axArrow, axRuntime, cat)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestList_StoreError_ReturnsError(t *testing.T) {
	svc, _ := testCatalog(t)

	svc.store = &failingArrowCatalog{listErr: errors.New("db unavailable")}

	result, err := svc.List(context.Background())
	require.Error(t, err)
	assert.Nil(t, result)
}

func TestUpdate_AxArrowExistsError_ReturnsError(t *testing.T) {
	svc, cat := testCatalog(t)

	svc.axArrow = &failingAxArrow{existsErr: errors.New("storage failure")}

	err := cat.Update(context.Background(), "github.com/org/repo", makeManifest("x"))
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_AxArrowExistsError_ReturnsError(t *testing.T) {
	svc, cat := testCatalog(t)

	svc.axArrow = &failingAxArrow{existsErr: errors.New("storage failure")}

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
}

func TestGet_AxArrowGetError_ReturnsError(t *testing.T) {
	svc, cat := testCatalog(t)

	svc.axArrow = &failingAxArrow{getErr: errors.New("storage failure")}

	got, err := cat.Get(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
	assert.Nil(t, got)
}

func TestUpdate_RuntimeGetError_ReturnsNoError_WhenAbsent(t *testing.T) {
	m := makeManifest("Arrow")
	updated := makeManifest("Updated")
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	// Runtime is not seeded -> absent state -> update allowed.
	require.NoError(t, cat.Update(context.Background(), "github.com/org/repo", updated))
}

func TestAdd_StoresManifestAndEmitsEvent(t *testing.T) {
	svc, _ := testCatalog(t)

	ns := domain.Namespace("github.com/user/repo")
	m := makeManifest("test-arrow")

	err := svc.Add(context.Background(), ns, m, false, "")
	require.NoError(t, err)

	svc.axArrow.WaitPublish()
	got, err := svc.axArrow.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, "test-arrow", got.Name)
}

func TestUpdate_Success_UpdatesManifestInAsynx(t *testing.T) {
	m := makeManifest("original")
	updated := makeManifest("updated")
	svc, cat := testCatalog(t)
	seedArrow(t, svc, "github.com/user/repo", m)

	err := cat.Update(context.Background(), "github.com/user/repo", updated)
	require.NoError(t, err)
	svc.axArrow.WaitPublish()

	got, err := svc.axArrow.Get(context.Background(), "github.com/user/repo")
	require.NoError(t, err)
	assert.Equal(t, "updated", got.Name)
}

func TestUpdate_NotFound_ReturnsErrNotFound2(t *testing.T) {
	_, cat := testCatalog(t)

	err := cat.Update(context.Background(), "github.com/user/repo", makeManifest("x"))
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

// failingAxArrow is a stub asynx.Asynx[domain.Arrow] that fails on the Nth Subscribe call
// and can optionally return an error from Get, Send, Exists, or Forget.
type failingAxArrow struct {
	subscribeCallN int
	calls          int
	err            error
	getErr         error
	existsErr      error
	existsReturn   bool
	onForgetErr    error
	sendErr        error
	forgetErr      error
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
	if f.sendErr != nil {
		return asynxModels.Event[domain.Arrow]{}, f.sendErr
	}
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
	return f.existsReturn, nil
}
func (f *failingAxArrow) Preload(_ context.Context, _ string) error { return nil }
func (f *failingAxArrow) Unsubscribe(_ string) error                { return nil }
func (f *failingAxArrow) Replay(_ context.Context, _ string, _ int64, _ int64, _ asynxModels.ProjectionHandler[domain.Arrow]) error {
	return nil
}
func (f *failingAxArrow) WaitPublish()                             {}
func (f *failingAxArrow) Forget(_ context.Context, _ string) error { return f.forgetErr }
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

	cat, err := New(&failingArrowAsynx{err: wantErr}, axRuntime, s)

	assert.Nil(t, cat)
	require.ErrorIs(t, err, wantErr)
}

// failingArrowCatalog is a store stub that can fail on List or Delete.
type failingArrowCatalog struct {
	listErr   error
	deleteErr error
}

func (f *failingArrowCatalog) Save(_ context.Context, _ domain.Arrow) error { return nil }
func (f *failingArrowCatalog) Delete(_ context.Context, _ domain.Namespace) error {
	return f.deleteErr
}
func (f *failingArrowCatalog) Get(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
	return nil, nil
}
func (f *failingArrowCatalog) List(_ context.Context) ([]domain.Arrow, error) {
	return nil, f.listErr
}
func (f *failingArrowCatalog) ListVersions(_ context.Context, _ domain.Namespace) ([]domain.Arrow, error) {
	return nil, nil
}

// failingAxRuntime is a minimal asynx.Asynx[domainRuntime.ArrowRuntime] stub
// whose Get returns getErr and Exists returns existsErr.
type failingAxRuntime struct {
	getErr    error
	existsErr error
}

func (f *failingAxRuntime) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
	_ ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime],
) (string, error) {
	return "sub-id", nil
}
func (f *failingAxRuntime) Send(
	_ context.Context,
	_ asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}
func (f *failingAxRuntime) SendWait(
	_ context.Context,
	_ asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}
func (f *failingAxRuntime) Get(
	_ context.Context,
	_ string,
) (domainRuntime.ArrowRuntime, error) {
	return domainRuntime.ArrowRuntime{}, f.getErr
}
func (f *failingAxRuntime) Exists(
	_ context.Context,
	_ string,
) (bool, error) {
	return false, f.existsErr
}
func (f *failingAxRuntime) Preload(_ context.Context, _ string) error { return nil }
func (f *failingAxRuntime) Unsubscribe(_ string) error                { return nil }
func (f *failingAxRuntime) Replay(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
) error {
	return nil
}
func (f *failingAxRuntime) Shutdown(_ context.Context) error         { return nil }
func (f *failingAxRuntime) WaitPublish()                             {}
func (f *failingAxRuntime) Forget(_ context.Context, _ string) error { return nil }
func (f *failingAxRuntime) OnForget(
	_ asynxModels.ForgetHandler[domainRuntime.ArrowRuntime],
) (string, error) {
	return "forget-sub-id", nil
}

func TestAdd_SendError_ReturnsWrappedError(t *testing.T) {
	sendErr := errors.New("send failure")
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t)

	svc.axArrow = &failingAxArrow{
		sendErr: sendErr,
		getErr:  asynxModels.ErrNotFound,
	}

	err := cat.Add(context.Background(), "github.com/org/repo", m, true, "")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrAlreadyExists)
	assert.ErrorContains(t, err, "add arrow")
}

func TestAdd_PipelineFailedOnSend_ReturnsErrAlreadyExists(t *testing.T) {
	// ErrPipelineFailed is returned by asynx on version conflicts (optimistic
	// concurrency), which happen when two goroutines concurrently add the same
	// arrow. The second write sees a stale version and gets a pipeline failure.
	// catalog.Add must map this to ErrAlreadyExists, not propagate it as 500.
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t)

	svc.axArrow = &failingAxArrow{
		sendErr: asynxModels.ErrPipelineFailed,
		getErr:  asynxModels.ErrNotFound,
	}

	err := cat.Add(context.Background(), "github.com/org/repo", m, true, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrAlreadyExists)
}

func TestUpdate_SendError_ReturnsWrappedError(t *testing.T) {
	sendErr := errors.New("send failure")
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	// existsReturn=true so Exists passes; sendErr causes Send to return a generic error.
	svc.axArrow = &failingAxArrow{
		sendErr:      sendErr,
		existsReturn: true,
	}

	err := cat.Update(context.Background(), "github.com/org/repo", makeManifest("updated"))
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperrors.ErrNotFound)
	assert.ErrorContains(t, err, "update arrow")
}

func TestUpdate_RuntimeGetError_NonNotFound_ReturnsError(t *testing.T) {
	runtimeErr := errors.New("runtime db failure")
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	svc.axRuntime = &failingAxRuntime{getErr: runtimeErr}

	err := cat.Update(context.Background(), "github.com/org/repo", makeManifest("x"))
	require.Error(t, err)
	assert.ErrorContains(t, err, "update arrow")
}

func TestUpdate_SendReturnsErrNotFound_ReturnsErrNotFound(t *testing.T) {
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	// existsReturn=true so Exists passes; sendErr causes Send to return ErrNotFound.
	svc.axArrow = &failingAxArrow{
		sendErr:      asynxModels.ErrNotFound,
		existsReturn: true,
	}

	err := cat.Update(context.Background(), "github.com/org/repo", makeManifest("x"))
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestRemove_RuntimeGetError_NonNotFound_ReturnsError(t *testing.T) {
	runtimeErr := errors.New("runtime db failure")
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	svc.axRuntime = &failingAxRuntime{getErr: runtimeErr}

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorContains(t, err, "remove arrow")
}

func TestRemove_ArrowMidInstall_NoRuntime_ReturnsStateViolation(t *testing.T) {
	svc, cat := testCatalog(t)
	ns := domain.Namespace("github.com/org/mid-install")

	// Seed arrow with InstalledRef set but InstalledAt zero (mid-install state).
	_, err := svc.axArrow.Send(context.Background(), midInstallArrowCmd{ns: ns})
	require.NoError(t, err)
	svc.axArrow.WaitPublish()

	// No runtime aggregate exists.
	err = cat.Remove(context.Background(), ns)
	require.Error(t, err)
	assert.ErrorIs(t, err, apperrors.ErrStateViolation)
}

func TestRemove_ArrowMidInstallButRuntimeReady_Allows(t *testing.T) {
	svc, cat := testCatalog(t)
	ns := domain.Namespace("github.com/org/mid-install-ready")

	// Seed arrow with InstalledRef set but InstalledAt zero (mid-install state).
	_, err := svc.axArrow.Send(context.Background(), midInstallArrowCmd{ns: ns})
	require.NoError(t, err)
	svc.axArrow.WaitPublish()

	// Runtime exists with State=ArrowStateReady.
	_, err = svc.axRuntime.Send(context.Background(), mocks.RuntimeCmd{
		NS:    ns,
		State: domain.ArrowStateReady,
	})
	require.NoError(t, err)
	svc.axRuntime.WaitPublish()

	// Remove should succeed: runtime says ready, stamp was missed but install completed.
	err = cat.Remove(context.Background(), ns)
	require.NoError(t, err)
}

func TestRemove_ForgetError_ReturnsError(t *testing.T) {
	forgetErr := errors.New("forget failure")
	m := makeManifest("Arrow")
	svc, cat := testCatalog(t)

	seedArrow(t, svc, "github.com/org/repo", m)

	// existsReturn=true so Exists passes; forgetErr causes Forget to fail.
	svc.axArrow = &failingAxArrow{
		forgetErr:    forgetErr,
		existsReturn: true,
	}

	err := cat.Remove(context.Background(), "github.com/org/repo")
	require.Error(t, err)
	assert.ErrorContains(t, err, "remove arrow")
}

// midInstallArrowCmd seeds an Arrow with InstalledRef set but InstalledAt zero,
// simulating a mid-install state where MarkInstalled has not yet been dispatched.
type midInstallArrowCmd struct {
	ns domain.Namespace
}

func (c midInstallArrowCmd) AggregateID() string            { return c.ns.String() }
func (c midInstallArrowCmd) EventName() string              { return "arrow.added" }
func (c midInstallArrowCmd) ShouldSnapshot() bool           { return false }
func (c midInstallArrowCmd) Validate(_ *domain.Arrow) error { return nil }
func (c midInstallArrowCmd) EmitEvent(_ *domain.Arrow) domain.Arrow {
	return domain.Arrow{
		Namespace:    c.ns,
		ArrowMeta:    domain.ArrowMeta{Name: "test", Version: "1.0.0"},
		InstalledRef: "v1.0.0",
		// InstalledAt intentionally left zero
	}
}
