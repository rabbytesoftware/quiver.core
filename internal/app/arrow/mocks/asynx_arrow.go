package mocks

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

// AsynxArrow mocks asynx.Asynx[domain.Arrow].
type AsynxArrow struct {
	GetValue    domain.Arrow
	GetErr      error
	ExistsValue bool
	ExistsErr   error
	SendErr     error
	SendFn      func() error // overrides SendErr when set
	ShutdownErr error
	ForgetErr   error
}

func (m *AsynxArrow) Send(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	if m.SendFn != nil {
		return asynxModels.Event[domain.Arrow]{}, m.SendFn()
	}
	return asynxModels.Event[domain.Arrow]{}, m.SendErr
}

func (m *AsynxArrow) SendWait(ctx context.Context, cmd asynxModels.Command[domain.Arrow]) (asynxModels.Event[domain.Arrow], error) {
	if m.SendFn != nil {
		return asynxModels.Event[domain.Arrow]{}, m.SendFn()
	}
	return asynxModels.Event[domain.Arrow]{}, m.SendErr
}

func (m *AsynxArrow) Get(ctx context.Context, aggregateID string) (domain.Arrow, error) {
	return m.GetValue, m.GetErr
}

func (m *AsynxArrow) Exists(ctx context.Context, aggregateID string) (bool, error) {
	return m.ExistsValue, m.ExistsErr
}

func (m *AsynxArrow) Preload(ctx context.Context, aggregateID string) error {
	return nil
}

func (m *AsynxArrow) Forget(ctx context.Context, aggregateID string) error {
	return m.ForgetErr
}

func (m *AsynxArrow) OnForget(fn asynxModels.ForgetHandler[domain.Arrow]) (string, error) {
	return "", nil
}

func (m *AsynxArrow) Subscribe(pattern string, handler asynxModels.ProjectionHandler[domain.Arrow], opts ...asynxModels.SubscriptionOpt[domain.Arrow]) (string, error) {
	return "", nil
}

func (m *AsynxArrow) Unsubscribe(id string) error {
	return nil
}

func (m *AsynxArrow) Replay(ctx context.Context, aggregateID string, fromVersion, toVersion int64, fn asynxModels.ProjectionHandler[domain.Arrow]) error {
	return nil
}

func (m *AsynxArrow) Shutdown(ctx context.Context) error {
	return m.ShutdownErr
}

func (m *AsynxArrow) WaitPublish() {}
