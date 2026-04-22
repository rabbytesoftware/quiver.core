package mocks

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// AsynxRuntime mocks asynx.Asynx[domainRuntime.ArrowRuntime].
type AsynxRuntime struct {
	GetValue    domainRuntime.ArrowRuntime
	GetErr      error
	ExistsValue bool
	ExistsErr   error
	PreloadErr  error
	ShutdownErr error
}

func (m *AsynxRuntime) Send(ctx context.Context, cmd asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (m *AsynxRuntime) SendWait(ctx context.Context, cmd asynxModels.Command[domainRuntime.ArrowRuntime]) (asynxModels.Event[domainRuntime.ArrowRuntime], error) {
	return asynxModels.Event[domainRuntime.ArrowRuntime]{}, nil
}

func (m *AsynxRuntime) Get(ctx context.Context, aggregateID string) (domainRuntime.ArrowRuntime, error) {
	return m.GetValue, m.GetErr
}

func (m *AsynxRuntime) Exists(ctx context.Context, aggregateID string) (bool, error) {
	return m.ExistsValue, m.ExistsErr
}

func (m *AsynxRuntime) Preload(ctx context.Context, aggregateID string) error {
	return m.PreloadErr
}

func (m *AsynxRuntime) Forget(ctx context.Context, aggregateID string) error {
	return nil
}

func (m *AsynxRuntime) OnForget(fn asynxModels.ForgetHandler[domainRuntime.ArrowRuntime]) (string, error) {
	return "", nil
}

func (m *AsynxRuntime) Subscribe(pattern string, handler asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime], opts ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime]) (string, error) {
	return "", nil
}

func (m *AsynxRuntime) Unsubscribe(id string) error {
	return nil
}

func (m *AsynxRuntime) Replay(ctx context.Context, aggregateID string, fromVersion, toVersion int64, fn asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime]) error {
	return nil
}

func (m *AsynxRuntime) Shutdown(ctx context.Context) error {
	return m.ShutdownErr
}

func (m *AsynxRuntime) WaitPublish() {}
