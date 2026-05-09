package mocks

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// AsynxRuntime is a mock for asynx.Asynx[domainRuntime.ArrowRuntime].
type AsynxRuntime struct {
	SendFn        func(ctx context.Context, cmd asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error)
	SendWaitFn    func(ctx context.Context, cmd asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error)
	ForgetFn      func(ctx context.Context, aggregateID string) error
	OnForgetFn    func(fn asynxModels.ForgetHandler[domainRuntime.ArrowRuntime]) (string, error)
	ShutdownFn    func(ctx context.Context) error
	GetFn         func(ctx context.Context, aggregateID string) (domainRuntime.ArrowRuntime, error)
	ExistsFn      func(ctx context.Context, aggregateID string) (bool, error)
	PreloadFn     func(ctx context.Context, aggregateID string) error
	SubscribeFn   func(pattern string, handler asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime], opts ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime]) (string, error)
	UnsubscribeFn func(id string) error
	ListenFn      func(pattern string, count int) (<-chan asynxModels.Event[domainRuntime.ArrowRuntime], func(), error)
	ReplayFn      func(ctx context.Context, aggregateID string, fromVersion, toVersion int64, fn asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime]) error
	WaitPublishFn func()
}

func (m *AsynxRuntime) Send(
	ctx context.Context,
	cmd asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	if m.SendFn != nil {
		return m.SendFn(ctx, cmd)
	}
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (m *AsynxRuntime) SendWait(
	ctx context.Context,
	cmd asynxModels.Command[domainRuntime.ArrowRuntime],
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	if m.SendWaitFn != nil {
		return m.SendWaitFn(ctx, cmd)
	}
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (m *AsynxRuntime) Forget(
	ctx context.Context,
	aggregateID string,
) error {
	if m.ForgetFn != nil {
		return m.ForgetFn(ctx, aggregateID)
	}
	return nil
}

func (m *AsynxRuntime) OnForget(
	fn asynxModels.ForgetHandler[domainRuntime.ArrowRuntime],
) (string, error) {
	if m.OnForgetFn != nil {
		return m.OnForgetFn(fn)
	}
	return "sub-id", nil
}

func (m *AsynxRuntime) Shutdown(ctx context.Context) error {
	if m.ShutdownFn != nil {
		return m.ShutdownFn(ctx)
	}
	return nil
}

func (m *AsynxRuntime) Get(
	ctx context.Context,
	aggregateID string,
) (domainRuntime.ArrowRuntime, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, aggregateID)
	}
	return domainRuntime.ArrowRuntime{}, asynxModels.ErrNotFound
}

func (m *AsynxRuntime) Exists(
	ctx context.Context,
	aggregateID string,
) (bool, error) {
	if m.ExistsFn != nil {
		return m.ExistsFn(ctx, aggregateID)
	}
	return false, nil
}

func (m *AsynxRuntime) Preload(
	ctx context.Context,
	aggregateID string,
) error {
	if m.PreloadFn != nil {
		return m.PreloadFn(ctx, aggregateID)
	}
	return nil
}

func (m *AsynxRuntime) Subscribe(
	pattern string,
	handler asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
	opts ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime],
) (string, error) {
	if m.SubscribeFn != nil {
		return m.SubscribeFn(pattern, handler, opts...)
	}
	return "sub-id", nil
}

func (m *AsynxRuntime) Unsubscribe(id string) error {
	if m.UnsubscribeFn != nil {
		return m.UnsubscribeFn(id)
	}
	return nil
}

func (m *AsynxRuntime) Listen(
	pattern string,
	count int,
) (<-chan asynxModels.Event[domainRuntime.ArrowRuntime], func(), error) {
	if m.ListenFn != nil {
		return m.ListenFn(pattern, count)
	}
	ch := make(chan asynxModels.Event[domainRuntime.ArrowRuntime], 1)
	close(ch)
	return ch, func() {}, nil
}

func (m *AsynxRuntime) SubscribeWait(
	ctx context.Context,
	pattern string,
) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	ch, unsub, err := m.Listen(pattern, 1)
	if err != nil {
		return asynxModels.Event[domainRuntime.ArrowRuntime]{}, err
	}
	defer unsub()
	select {
	case evt := <-ch:
		return evt, nil
	case <-ctx.Done():
		return asynxModels.Event[domainRuntime.ArrowRuntime]{}, ctx.Err()
	}
}

func (m *AsynxRuntime) Replay(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
	toVersion int64,
	fn asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
) error {
	if m.ReplayFn != nil {
		return m.ReplayFn(ctx, aggregateID, fromVersion, toVersion, fn)
	}
	return nil
}

func (m *AsynxRuntime) WaitPublish() {
	if m.WaitPublishFn != nil {
		m.WaitPublishFn()
	}
}
