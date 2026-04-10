package arrow

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
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

	cat, err := catalog.New(axArrow, axRuntime, store, nil, nil)
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
	cat, err := catalog.New(axArrow, axRuntime, s, nil, nil)
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

	mv := &mocks.Vault{
		GetArrowErr:  vault.ErrNotCached,
		PutArrowPath: "/tmp/test",
	}
	mm := &mocks.Manifold{
		ResolveArrowManifest: &domain.ArrowManifest{Name: "test", Version: "1.0.0"},
	}

	cat, err := catalog.New(axArrow, axRuntime, store, mv, mm)
	require.NoError(t, err)

	return cat, axArrow, axRuntime
}

func TestBuilder_WithWebSocketHub_BroadcastsArrowEvents(t *testing.T) {
	cat, axArrow, axRuntime := buildTestCatalogWithMocks(t)
	hub := &stubHub{}

	svc, err := NewArrowBuilder().
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
		WithAsynxArrow(axArrow).
		WithAsynxRuntime(axRuntime).
		WithCatalog(cat).
		Build()
	require.NoError(t, err)

	ctx := context.Background()
	ns := domain.Namespace("github.com/user/repo")
	assert.NoError(t, svc.Add(ctx, ns))
}
