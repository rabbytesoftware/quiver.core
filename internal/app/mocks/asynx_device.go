package mocks

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// AsynxDevice is a mock for asynx.Asynx[auth.Device].
type AsynxDevice struct {
	SendFn        func(ctx context.Context, cmd asynxModels.Command[auth.Device]) (asynxModels.Event[auth.Device], error)
	SendWaitFn    func(ctx context.Context, cmd asynxModels.Command[auth.Device]) (asynxModels.Event[auth.Device], error)
	ForgetFn      func(ctx context.Context, aggregateID string) error
	OnForgetFn    func(fn asynxModels.ForgetHandler[auth.Device]) (string, error)
	ShutdownFn    func(ctx context.Context) error
	GetFn         func(ctx context.Context, aggregateID string) (auth.Device, error)
	ExistsFn      func(ctx context.Context, aggregateID string) (bool, error)
	PreloadFn     func(ctx context.Context, aggregateID string) error
	SubscribeFn   func(pattern string, handler asynxModels.ProjectionHandler[auth.Device], opts ...asynxModels.SubscriptionOpt[auth.Device]) (string, error)
	UnsubscribeFn func(id string) error
	ListenFn      func(pattern string, count int) (<-chan asynxModels.Event[auth.Device], func(), error)
	ReplayFn      func(ctx context.Context, aggregateID string, fromVersion, toVersion int64, fn asynxModels.ProjectionHandler[auth.Device]) error
	WaitPublishFn func()
}

func (m *AsynxDevice) Send(
	ctx context.Context,
	cmd asynxModels.Command[auth.Device],
) (asynxModels.Event[auth.Device], error) {
	if m.SendFn != nil {
		return m.SendFn(ctx, cmd)
	}
	return asynxModels.Event[auth.Device]{}, nil
}

func (m *AsynxDevice) SendWait(
	ctx context.Context,
	cmd asynxModels.Command[auth.Device],
) (asynxModels.Event[auth.Device], error) {
	if m.SendWaitFn != nil {
		return m.SendWaitFn(ctx, cmd)
	}
	return asynxModels.Event[auth.Device]{}, nil
}

func (m *AsynxDevice) Forget(
	ctx context.Context,
	aggregateID string,
) error {
	if m.ForgetFn != nil {
		return m.ForgetFn(ctx, aggregateID)
	}
	return nil
}

func (m *AsynxDevice) OnForget(
	fn asynxModels.ForgetHandler[auth.Device],
) (string, error) {
	if m.OnForgetFn != nil {
		return m.OnForgetFn(fn)
	}
	return "sub-id", nil
}

func (m *AsynxDevice) Shutdown(ctx context.Context) error {
	if m.ShutdownFn != nil {
		return m.ShutdownFn(ctx)
	}
	return nil
}

func (m *AsynxDevice) Get(
	ctx context.Context,
	aggregateID string,
) (auth.Device, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, aggregateID)
	}
	return auth.Device{}, asynxModels.ErrNotFound
}

func (m *AsynxDevice) Exists(
	ctx context.Context,
	aggregateID string,
) (bool, error) {
	if m.ExistsFn != nil {
		return m.ExistsFn(ctx, aggregateID)
	}
	return false, nil
}

func (m *AsynxDevice) Preload(
	ctx context.Context,
	aggregateID string,
) error {
	if m.PreloadFn != nil {
		return m.PreloadFn(ctx, aggregateID)
	}
	return nil
}

func (m *AsynxDevice) Subscribe(
	pattern string,
	handler asynxModels.ProjectionHandler[auth.Device],
	opts ...asynxModels.SubscriptionOpt[auth.Device],
) (string, error) {
	if m.SubscribeFn != nil {
		return m.SubscribeFn(pattern, handler, opts...)
	}
	return "sub-id", nil
}

func (m *AsynxDevice) Unsubscribe(id string) error {
	if m.UnsubscribeFn != nil {
		return m.UnsubscribeFn(id)
	}
	return nil
}

func (m *AsynxDevice) Listen(
	pattern string,
	count int,
) (<-chan asynxModels.Event[auth.Device], func(), error) {
	if m.ListenFn != nil {
		return m.ListenFn(pattern, count)
	}
	ch := make(chan asynxModels.Event[auth.Device], 1)
	close(ch)
	return ch, func() {}, nil
}

func (m *AsynxDevice) SubscribeWait(
	ctx context.Context,
	pattern string,
) (asynxModels.Event[auth.Device], error) {
	ch, unsub, err := m.Listen(pattern, 1)
	if err != nil {
		return asynxModels.Event[auth.Device]{}, err
	}
	defer unsub()
	select {
	case evt := <-ch:
		return evt, nil
	case <-ctx.Done():
		return asynxModels.Event[auth.Device]{}, ctx.Err()
	}
}

func (m *AsynxDevice) Replay(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
	toVersion int64,
	fn asynxModels.ProjectionHandler[auth.Device],
) error {
	if m.ReplayFn != nil {
		return m.ReplayFn(ctx, aggregateID, fromVersion, toVersion, fn)
	}
	return nil
}

func (m *AsynxDevice) WaitPublish() {
	if m.WaitPublishFn != nil {
		m.WaitPublishFn()
	}
}
