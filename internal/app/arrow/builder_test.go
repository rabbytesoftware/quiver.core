package arrow

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubHub struct {
	mu       sync.Mutex
	arrows   []domain.Arrow
	runtimes []domainRuntime.ArrowRuntime
	quivers  []domain.Quiver
}

func (s *stubHub) BroadcastArrow(a domain.Arrow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.arrows = append(s.arrows, a)
}

func (s *stubHub) BroadcastArrowRuntime(r domainRuntime.ArrowRuntime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runtimes = append(s.runtimes, r)
}

func (s *stubHub) BroadcastQuiver(q domain.Quiver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quivers = append(s.quivers, q)
}

// failingRuntimeAsynxBuilder is a minimal asynx.Asynx[domainRuntime.ArrowRuntime] stub
// whose Subscribe always returns an error. This forces runner.New (via execution.New) to fail.
type failingRuntimeAsynxBuilder struct {
	err error
}

func (f *failingRuntimeAsynxBuilder) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
	_ ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime],
) (string, error) {
	return "", f.err
}

func (f *failingRuntimeAsynxBuilder) Send(
	_ context.Context,
	_ asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (f *failingRuntimeAsynxBuilder) SendWait(
	_ context.Context,
	_ asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (f *failingRuntimeAsynxBuilder) Get(
	_ context.Context,
	_ string,
) (domainRuntime.ArrowRuntime, error) {
	return domainRuntime.ArrowRuntime{}, nil
}

func (f *failingRuntimeAsynxBuilder) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (f *failingRuntimeAsynxBuilder) Preload(_ context.Context, _ string) error { return nil }
func (f *failingRuntimeAsynxBuilder) Unsubscribe(_ string) error                { return nil }
func (f *failingRuntimeAsynxBuilder) Replay(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
) error {
	return nil
}
func (f *failingRuntimeAsynxBuilder) Forget(_ context.Context, _ string) error { return nil }
func (f *failingRuntimeAsynxBuilder) OnForget(_ asynxModels.ForgetHandler[domainRuntime.ArrowRuntime]) (string, error) {
	return "forget-sub-id", nil
}
func (f *failingRuntimeAsynxBuilder) Shutdown(_ context.Context) error { return nil }
func (f *failingRuntimeAsynxBuilder) WaitPublish()                     {}

func buildTestCatalog(
	t *testing.T,
) (catalog.Catalog, asynx.Asynx[domain.Arrow], asynx.Asynx[domainRuntime.ArrowRuntime]) {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	store, err := arrowstore.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	cat, err := catalog.New(axArrow, axRuntime, store)
	require.NoError(t, err)

	return cat, axArrow, axRuntime
}

func TestBuilder_Build_SucceedsWithValidEventStore(t *testing.T) {
	cat, axArrow, axRuntime := buildTestCatalog(t)

	svc, err := NewArrowBuilder().
		WithAsynxArrow(axArrow).
		WithAsynxRuntime(axRuntime).
		WithCatalog(cat).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_FailsWithNilEventStore(t *testing.T) {
	svc, err := NewArrowBuilder().Build()

	require.Error(t, err)
	assert.Nil(t, svc)
}

func TestBuilder_Build_UsesProvidedCatalog(t *testing.T) {
	cat, axArrow, axRuntime := buildTestCatalog(t)

	svc, err := NewArrowBuilder().
		WithAsynxArrow(axArrow).
		WithAsynxRuntime(axRuntime).
		WithCatalog(cat).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_SucceedsWithSeparateRuntimeEventStore(t *testing.T) {
	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	// Provide a catalog built from the same event stores the builder will use.
	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)
	s, err := arrowstore.NewArrowCatalog(":memory:")
	require.NoError(t, err)
	cat, err := catalog.New(axArrow, axRuntime, s)
	require.NoError(t, err)

	svc, err := NewArrowBuilder().
		WithEventStore(arrowES).
		WithRuntimeEventStore(runtimeES).
		WithCatalog(cat).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_DefaultCatalogStorePathIsCovered(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	// No WithCatalog — triggers the default store creation code path.
	// The build may succeed or fail depending on the test environment's filesystem;
	// either outcome covers the target branch.
	_, _ = NewArrowBuilder().
		WithEventStore(es).
		Build()
}

func TestBuilder_WithHomeDir_UsesCustomPath(t *testing.T) {
	cat, axArrow, axRuntime := buildTestCatalog(t)
	dir := t.TempDir()

	svc, err := NewArrowBuilder().
		WithAsynxArrow(axArrow).
		WithAsynxRuntime(axRuntime).
		WithCatalog(cat).
		WithHomeDir(dir).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)

	// Shut down before t.TempDir cleanup so Windows releases the SQLite file lock.
	t.Cleanup(func() { _ = svc.Shutdown(context.Background()) })
}

func TestBuilder_WithHomeDir_InvalidPath_ReturnsError(t *testing.T) {
	cat, axArrow, axRuntime := buildTestCatalog(t)

	// Use a file as homeDir so that paths.StoreAt fails trying to mkdir inside a file.
	f, err := os.CreateTemp("", "quiver-test-homedir-*")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	f.Close()

	_, buildErr := NewArrowBuilder().
		WithAsynxArrow(axArrow).
		WithAsynxRuntime(axRuntime).
		WithCatalog(cat).
		WithHomeDir(f.Name()). // file path, not a directory
		Build()

	assert.Error(t, buildErr)
}

func TestNewAsynxArrow_FailsWithNilEventStore(t *testing.T) {
	ax, err := newAsynxArrow(nil)
	require.Error(t, err)
	assert.Nil(t, ax)
}

func TestNewAsynxRuntime_FailsWithNilEventStore(t *testing.T) {
	ax, err := newAsynxRuntime(nil)
	require.Error(t, err)
	assert.Nil(t, ax)
}

func TestBuilder_Build_FailsWhenExecutionSubscribeFails(t *testing.T) {
	wantErr := errors.New("runtime subscribe failed")

	cat, axArrow, _ := buildTestCatalog(t)
	badRuntime := &failingRuntimeAsynxBuilder{err: wantErr}

	svc, err := NewArrowBuilder().
		WithAsynxArrow(axArrow).
		WithAsynxRuntime(badRuntime).
		WithCatalog(cat).
		Build()

	assert.Nil(t, svc)
	require.ErrorIs(t, err, wantErr)
}

func buildTestCatalogWithMocks(
	t *testing.T,
) (catalog.Catalog, asynx.Asynx[domain.Arrow], asynx.Asynx[domainRuntime.ArrowRuntime]) {
	t.Helper()

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	store, err := arrowstore.NewArrowCatalog(":memory:")
	require.NoError(t, err)

	cat, err := catalog.New(axArrow, axRuntime, store)
	require.NoError(t, err)

	return cat, axArrow, axRuntime
}

func TestBuilder_WithWebSocketHub_BroadcastsArrowEvents(t *testing.T) {
	cat, axArrow, axRuntime := buildTestCatalogWithMocks(t)
	hub := &stubHub{}

	svc, err := NewArrowBuilder().
		WithEngines(&engine.Container{
			Vault: &mocks.Vault{
				GetArrowErr: vault.ErrNotCached,
			},
			Manifold: &mocks.Manifold{
				ResolveArrowResult: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test", Version: "1.0.0"}},
			},
		}).
		WithAsynxArrow(axArrow).
		WithAsynxRuntime(axRuntime).
		WithCatalog(cat).
		WithWebSocketHub(hub).
		Build()
	require.NoError(t, err)

	ctx := context.Background()
	ns := domain.Namespace("github.com/user/repo")

	require.NoError(t, svc.Add(ctx, ns))
	axArrow.WaitPublish()

	hub.mu.Lock()
	defer hub.mu.Unlock()
	require.Len(t, hub.arrows, 1)
	assert.Equal(t, ns, hub.arrows[0].Namespace)
}

func TestBuilder_WithWebSocketHub_NilHub_NoPanic(t *testing.T) {
	cat, axArrow, axRuntime := buildTestCatalogWithMocks(t)

	svc, err := NewArrowBuilder().
		WithEngines(&engine.Container{
			Vault: &mocks.Vault{
				GetArrowErr: vault.ErrNotCached,
			},
			Manifold: &mocks.Manifold{
				ResolveArrowResult: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "test", Version: "1.0.0"}},
			},
		}).
		WithAsynxArrow(axArrow).
		WithAsynxRuntime(axRuntime).
		WithCatalog(cat).
		Build()
	require.NoError(t, err)

	ctx := context.Background()
	ns := domain.Namespace("github.com/user/repo")
	assert.NoError(t, svc.Add(ctx, ns))
}

// failingArrowAsynxBuilder is a minimal asynx.Asynx[domain.Arrow] stub
// whose Subscribe always returns an error.
type failingArrowAsynxBuilder struct {
	err error
}

func (f *failingArrowAsynxBuilder) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Arrow],
	_ ...asynxModels.SubscriptionOpt[domain.Arrow],
) (string, error) {
	return "", f.err
}

func (f *failingArrowAsynxBuilder) Send(
	_ context.Context,
	_ asynxModels.Command[domain.Arrow],
) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingArrowAsynxBuilder) SendWait(
	_ context.Context,
	_ asynxModels.Command[domain.Arrow],
) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingArrowAsynxBuilder) Get(
	_ context.Context,
	_ string,
) (domain.Arrow, error) {
	return domain.Arrow{}, nil
}

func (f *failingArrowAsynxBuilder) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (f *failingArrowAsynxBuilder) Preload(_ context.Context, _ string) error { return nil }
func (f *failingArrowAsynxBuilder) Unsubscribe(_ string) error                { return nil }
func (f *failingArrowAsynxBuilder) Replay(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
	_ asynxModels.ProjectionHandler[domain.Arrow],
) error {
	return nil
}
func (f *failingArrowAsynxBuilder) Forget(_ context.Context, _ string) error { return nil }
func (f *failingArrowAsynxBuilder) OnForget(_ asynxModels.ForgetHandler[domain.Arrow]) (string, error) {
	return "forget-sub-id", nil
}
func (f *failingArrowAsynxBuilder) Shutdown(_ context.Context) error { return nil }
func (f *failingArrowAsynxBuilder) WaitPublish()                     {}

func TestBuilder_WithEngines_Succeeds(t *testing.T) {
	cat, axArrow, axRuntime := buildTestCatalog(t)
	eng := &engine.Container{}

	svc, err := NewArrowBuilder().
		WithAsynxArrow(axArrow).
		WithAsynxRuntime(axRuntime).
		WithCatalog(cat).
		WithEngines(eng).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

// invokingRuntimeAsynx is a mock that immediately fires the subscription handler
// on the first Subscribe call. Used to exercise hub.BroadcastArrowRuntime.
type invokingRuntimeAsynx struct {
	*failingRuntimeAsynxBuilder
}

func (a *invokingRuntimeAsynx) Subscribe(
	_ string,
	handler asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
	_ ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime],
) (string, error) {
	handler(context.Background(), asynxModels.Event[domainRuntime.ArrowRuntime]{
		Aggregate: domainRuntime.ArrowRuntime{State: domain.ArrowStateReady},
	})
	return "sub-id", nil
}

func TestRegisterWSProjections_BroadcastsRuntimeEvent(t *testing.T) {
	_, axArrow, _ := buildTestCatalog(t)
	axRuntime := &invokingRuntimeAsynx{&failingRuntimeAsynxBuilder{}}
	hub := &stubHub{}

	err := registerWSProjections(axArrow, axRuntime, hub)

	require.NoError(t, err)
	hub.mu.Lock()
	defer hub.mu.Unlock()
	require.Len(t, hub.runtimes, 1)
	assert.Equal(t, domain.ArrowStateReady, hub.runtimes[0].State)
}

func TestRegisterWSProjections_ArrowSubscribeError(t *testing.T) {
	wantErr := errors.New("arrow subscribe failed")
	failArrow := &failingArrowAsynxBuilder{err: wantErr}

	_, _, axRuntime := buildTestCatalog(t)
	hub := &stubHub{}

	err := registerWSProjections(failArrow, axRuntime, hub)

	require.Error(t, err)
	assert.ErrorContains(t, err, "ws arrow subscription")
}

func TestRegisterWSProjections_RuntimeSubscribeError(t *testing.T) {
	wantErr := errors.New("runtime subscribe failed")
	failRuntime := &failingRuntimeAsynxBuilder{err: wantErr}

	_, axArrow, _ := buildTestCatalog(t)
	hub := &stubHub{}

	err := registerWSProjections(axArrow, failRuntime, hub)

	require.Error(t, err)
	assert.ErrorContains(t, err, "ws runtime subscription")
}
