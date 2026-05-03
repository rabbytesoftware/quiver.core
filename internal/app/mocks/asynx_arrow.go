package mocks

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// AsynxArrow is a mock for asynx.Asynx[domain.Arrow].
type AsynxArrow struct {
	SendFn        func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error)
	SendWaitFn    func(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error)
	ForgetFn      func(ctx context.Context, aggregateID string) error
	OnForgetFn    func(fn asynxModels.ForgetHandler[domain.Arrow]) (string, error)
	ShutdownFn    func(ctx context.Context) error
	GetFn         func(ctx context.Context, aggregateID string) (domain.Arrow, error)
	ExistsFn      func(ctx context.Context, aggregateID string) (bool, error)
	PreloadFn     func(ctx context.Context, aggregateID string) error
	SubscribeFn   func(pattern string, handler asynxModels.ProjectionHandler[domain.Arrow], opts ...asynxModels.SubscriptionOpt[domain.Arrow]) (string, error)
	UnsubscribeFn func(id string) error
	ListenFn      func(pattern string, count int) (<-chan asynxModels.Event[domain.Arrow], func(), error)
	ReplayFn      func(ctx context.Context, aggregateID string, fromVersion, toVersion int64, fn asynxModels.ProjectionHandler[domain.Arrow]) error
	WaitPublishFn func()
}

func (m *AsynxArrow) Send(
	ctx context.Context,
	cmd asynxModels.Command[domain.Arrow],
) (asynxModels.Event[domain.Arrow], error) {
	if m.SendFn != nil {
		return m.SendFn(ctx, cmd)
	}
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (m *AsynxArrow) SendWait(
	ctx context.Context,
	cmd asynxModels.Command[domain.Arrow],
) (asynxModels.Event[domain.Arrow], error) {
	if m.SendWaitFn != nil {
		return m.SendWaitFn(ctx, cmd)
	}
	return asynxModels.Event[domain.Arrow]{}, nil
}

func (m *AsynxArrow) Forget(
	ctx context.Context,
	aggregateID string,
) error {
	if m.ForgetFn != nil {
		return m.ForgetFn(ctx, aggregateID)
	}
	return nil
}

func (m *AsynxArrow) OnForget(
	fn asynxModels.ForgetHandler[domain.Arrow],
) (string, error) {
	if m.OnForgetFn != nil {
		return m.OnForgetFn(fn)
	}
	return "sub-id", nil
}

func (m *AsynxArrow) Shutdown(ctx context.Context) error {
	if m.ShutdownFn != nil {
		return m.ShutdownFn(ctx)
	}
	return nil
}

func (m *AsynxArrow) Get(
	ctx context.Context,
	aggregateID string,
) (domain.Arrow, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, aggregateID)
	}
	return domain.Arrow{}, asynxModels.ErrNotFound
}

func (m *AsynxArrow) Exists(
	ctx context.Context,
	aggregateID string,
) (bool, error) {
	if m.ExistsFn != nil {
		return m.ExistsFn(ctx, aggregateID)
	}
	return false, nil
}

func (m *AsynxArrow) Preload(
	ctx context.Context,
	aggregateID string,
) error {
	if m.PreloadFn != nil {
		return m.PreloadFn(ctx, aggregateID)
	}
	return nil
}

func (m *AsynxArrow) Subscribe(
	pattern string,
	handler asynxModels.ProjectionHandler[domain.Arrow],
	opts ...asynxModels.SubscriptionOpt[domain.Arrow],
) (string, error) {
	if m.SubscribeFn != nil {
		return m.SubscribeFn(pattern, handler, opts...)
	}
	return "sub-id", nil
}

func (m *AsynxArrow) Unsubscribe(id string) error {
	if m.UnsubscribeFn != nil {
		return m.UnsubscribeFn(id)
	}
	return nil
}

func (m *AsynxArrow) Listen(
	pattern string,
	count int,
) (<-chan asynxModels.Event[domain.Arrow], func(), error) {
	if m.ListenFn != nil {
		return m.ListenFn(pattern, count)
	}
	ch := make(chan asynxModels.Event[domain.Arrow], 1)
	close(ch)
	return ch, func() {}, nil
}

func (m *AsynxArrow) SubscribeWait(
	ctx context.Context,
	pattern string,
) (asynxModels.Event[domain.Arrow], error) {
	ch, unsub, err := m.Listen(pattern, 1)
	if err != nil {
		return asynxModels.Event[domain.Arrow]{}, err
	}
	defer unsub()
	select {
	case evt := <-ch:
		return evt, nil
	case <-ctx.Done():
		return asynxModels.Event[domain.Arrow]{}, ctx.Err()
	}
}

func (m *AsynxArrow) Replay(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
	toVersion int64,
	fn asynxModels.ProjectionHandler[domain.Arrow],
) error {
	if m.ReplayFn != nil {
		return m.ReplayFn(ctx, aggregateID, fromVersion, toVersion, fn)
	}
	return nil
}

func (m *AsynxArrow) WaitPublish() {
	if m.WaitPublishFn != nil {
		m.WaitPublishFn()
	}
}
