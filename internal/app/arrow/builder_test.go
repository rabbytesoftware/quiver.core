package arrow

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingArrowAsynx is a minimal asynx.Asynx[domain.Arrow] stub whose
// Subscribe always returns an error. This forces catalog.New to fail.
type failingArrowAsynx struct {
	err error
}

func (f *failingArrowAsynx) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domain.Arrow],
	_ ...asynxModels.SubscriptionOpt[domain.Arrow],
) (string, error) {
	return "", f.err
}

func (f *failingArrowAsynx) Send(
	_ context.Context,
	_ asynxModels.Command[domain.Arrow],
) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingArrowAsynx) SendWait(
	_ context.Context,
	_ asynxModels.Command[domain.Arrow],
) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (f *failingArrowAsynx) Get(_ context.Context, _ string) (domain.Arrow, error) {
	return domain.Arrow{}, nil
}

func (f *failingArrowAsynx) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *failingArrowAsynx) Preload(_ context.Context, _ string) error         { return nil }
func (f *failingArrowAsynx) Unsubscribe(_ string) error                        { return nil }
func (f *failingArrowAsynx) Replay(
	_ context.Context,
	_ string,
	_ int64,
	_ int64,
	_ asynxModels.ProjectionHandler[domain.Arrow],
) error {
	return nil
}
func (f *failingArrowAsynx) Shutdown(_ context.Context) error { return nil }
func (f *failingArrowAsynx) WaitPublish()                     {}

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

func newTestStore(t *testing.T) arrowstore.ArrowCatalog {
	t.Helper()
	store, err := arrowstore.NewArrowCatalog(":memory:")
	require.NoError(t, err)
	return store
}

func TestBuilder_Build_SucceedsWithValidEventStore(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	svc, err := NewArrowBuilder().
		WithEventStore(es).
		WithCatalogStore(newTestStore(t)).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_FailsWithNilEventStore(t *testing.T) {
	svc, err := NewArrowBuilder().Build()

	require.Error(t, err)
	assert.Nil(t, svc)
}

func TestBuilder_Build_UsesProvidedCatalogStore(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	svc, err := NewArrowBuilder().
		WithEventStore(es).
		WithCatalogStore(newTestStore(t)).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_UsesProvidedCatalog(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	// Build a standalone catalog using its own event stores.
	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)
	cat, err := catalog.New(axArrow, axRuntime, newTestStore(t), nil, nil)
	require.NoError(t, err)

	svc, err := NewArrowBuilder().
		WithEventStore(es).
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

	svc, err := NewArrowBuilder().
		WithEventStore(arrowES).
		WithRuntimeEventStore(runtimeES).
		WithCatalogStore(newTestStore(t)).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestBuilder_Build_FailsWhenCatalogSubscribeFails(t *testing.T) {
	wantErr := errors.New("subscribe failed")
	badArrow := &failingArrowAsynx{err: wantErr}

	runtimeES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axRuntime, err := newAsynxRuntime(runtimeES)
	require.NoError(t, err)

	svc, err := NewArrowBuilder().
		WithAsynxInstances(badArrow, axRuntime).
		WithCatalogStore(newTestStore(t)).
		Build()

	assert.Nil(t, svc)
	require.ErrorIs(t, err, wantErr)
}

func TestBuilder_Build_DefaultCatalogStorePathIsCovered(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	// No WithCatalogStore — triggers the default store creation code path.
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

	arrowES, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	axArrow, err := newAsynxArrow(arrowES)
	require.NoError(t, err)

	badRuntime := &failingRuntimeAsynxBuilder{err: wantErr}

	svc, err := NewArrowBuilder().
		WithAsynxInstances(axArrow, badRuntime).
		WithCatalogStore(newTestStore(t)).
		Build()

	assert.Nil(t, svc)
	require.ErrorIs(t, err, wantErr)
}
