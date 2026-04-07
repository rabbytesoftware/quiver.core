package projections

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
	"github.com/stretchr/testify/require"
)

// failingArrow is a stub asynx.Asynx[domain.Arrow] that returns an error on the
// Nth call to Subscribe (1-indexed). All other methods panic or no-op.
type failingArrow struct {
	subscribeCallN int // which call number should fail (1 = first)
	calls          int
	err            error
}

func (f *failingArrow) Subscribe(
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

func (f *failingArrow) Send(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}
func (f *failingArrow) SendWait(_ context.Context, _ asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	return asynxModels.Event[domain.Arrow]{}, nil
}
func (f *failingArrow) Shutdown(_ context.Context) error                              { return nil }
func (f *failingArrow) Get(_ context.Context, _ string) (domain.Arrow, error)         { return domain.Arrow{}, nil }
func (f *failingArrow) Exists(_ context.Context, _ string) (bool, error)              { return false, nil }
func (f *failingArrow) Preload(_ context.Context, _ string) error                     { return nil }
func (f *failingArrow) Unsubscribe(_ string) error                                    { return nil }
func (f *failingArrow) Replay(_ context.Context, _ string, _ int64, _ int64, _ asynxModels.ProjectionHandler[domain.Arrow]) error {
	return nil
}
func (f *failingArrow) WaitPublish() {}

// failingRuntime is a stub asynx.Asynx[domainRuntime.ArrowRuntime] that returns
// an error on the Nth Subscribe call.
type failingRuntime struct {
	subscribeCallN int
	calls          int
	err            error
}

func (f *failingRuntime) Subscribe(
	_ string,
	_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
	_ ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime],
) (string, error) {
	f.calls++
	if f.calls == f.subscribeCallN {
		return "", f.err
	}
	return "sub-id", nil
}

func (f *failingRuntime) Send(_ context.Context, _ asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}
func (f *failingRuntime) SendWait(_ context.Context, _ asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}
func (f *failingRuntime) Shutdown(_ context.Context) error { return nil }
func (f *failingRuntime) Get(_ context.Context, _ string) (domainRuntime.ArrowRuntime, error) {
	return domainRuntime.ArrowRuntime{}, nil
}
func (f *failingRuntime) Exists(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *failingRuntime) Preload(_ context.Context, _ string) error         { return nil }
func (f *failingRuntime) Unsubscribe(_ string) error                        { return nil }
func (f *failingRuntime) Replay(_ context.Context, _ string, _ int64, _ int64, _ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime]) error {
	return nil
}
func (f *failingRuntime) WaitPublish() {}

func TestInit_RegistersAllSubscriptions(t *testing.T) {
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)

	axArrow, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		Build()
	require.NoError(t, err)

	axRuntime, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		Build()
	require.NoError(t, err)

	catalog := &mockArrowCatalog{}
	var wiz wizard.Wizard
	executor := NewWizardExecutor(axRuntime, axArrow, wiz)

	err = Init(axArrow, axRuntime, catalog, wiz, executor)

	require.NoError(t, err)
}

// Init calls Subscribe in this order:
//  1. axArrow.Subscribe("arrow.added", ...)
//  2. axArrow.Subscribe("arrow.updated", ...)
//  3. axArrow.Subscribe("arrow.removed", ...)
//  4. axRuntime.Subscribe("runtime.mark_stopping", ...)
//  5. axRuntime.Subscribe("runtime.begun", ...)

func TestInit_ArrowAddedSubscribeError_ReturnsError(t *testing.T) {
	subErr := errors.New("subscribe failed")
	axArrow := &failingArrow{subscribeCallN: 1, err: subErr}
	axRuntime := &failingRuntime{}
	catalog := &mockArrowCatalog{}
	executor := NewWizardExecutor(axRuntime, axArrow, nil)

	err := Init(axArrow, axRuntime, catalog, nil, executor)

	require.Error(t, err)
	require.ErrorIs(t, err, subErr)
}

func TestInit_ArrowUpdatedSubscribeError_ReturnsError(t *testing.T) {
	subErr := errors.New("subscribe failed")
	axArrow := &failingArrow{subscribeCallN: 2, err: subErr}
	axRuntime := &failingRuntime{}
	catalog := &mockArrowCatalog{}
	executor := NewWizardExecutor(axRuntime, axArrow, nil)

	err := Init(axArrow, axRuntime, catalog, nil, executor)

	require.Error(t, err)
	require.ErrorIs(t, err, subErr)
}

func TestInit_ArrowRemovedSubscribeError_ReturnsError(t *testing.T) {
	subErr := errors.New("subscribe failed")
	axArrow := &failingArrow{subscribeCallN: 3, err: subErr}
	axRuntime := &failingRuntime{}
	catalog := &mockArrowCatalog{}
	executor := NewWizardExecutor(axRuntime, axArrow, nil)

	err := Init(axArrow, axRuntime, catalog, nil, executor)

	require.Error(t, err)
	require.ErrorIs(t, err, subErr)
}

func TestInit_RuntimeMarkStoppingSubscribeError_ReturnsError(t *testing.T) {
	subErr := errors.New("subscribe failed")
	axArrow := &failingArrow{}
	axRuntime := &failingRuntime{subscribeCallN: 1, err: subErr}
	catalog := &mockArrowCatalog{}
	executor := NewWizardExecutor(axRuntime, axArrow, nil)

	err := Init(axArrow, axRuntime, catalog, nil, executor)

	require.Error(t, err)
	require.ErrorIs(t, err, subErr)
}

func TestInit_RuntimeBegunSubscribeError_ReturnsError(t *testing.T) {
	subErr := errors.New("subscribe failed")
	axArrow := &failingArrow{}
	axRuntime := &failingRuntime{subscribeCallN: 2, err: subErr}
	catalog := &mockArrowCatalog{}
	executor := NewWizardExecutor(axRuntime, axArrow, nil)

	err := Init(axArrow, axRuntime, catalog, nil, executor)

	require.Error(t, err)
	require.ErrorIs(t, err, subErr)
}
