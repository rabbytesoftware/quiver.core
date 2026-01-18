package arrows

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/shared"
	"github.com/rabbytesoftware/quiver/internal/repositories"
	"github.com/rabbytesoftware/quiver/internal/repositories/arrows"
)

type ArrowUsecases struct {
	repository arrows.ArrowsInterface
	ctx        context.Context
}

func NewArrowsUsecases(repos *repositories.Repositories) *ArrowUsecases {

	return &ArrowUsecases{
		repository: repos.GetArrows(),
		ctx:        context.Background(),
	}
}

func (uc *ArrowUsecases) Add(
	searchValue,
	valueType,
	clientIP string,
	force bool,
) (arrow.Arrow, []error, error) {
	return uc.repository.Add(uc.ctx, searchValue, force, clientIP)
}

func (uc *ArrowUsecases) Remove(namespace, clientIP string) ([]error, error) {
	return uc.repository.Remove(uc.ctx, shared.Namespace(namespace), false, clientIP)
}

func (uc *ArrowUsecases) ExecuteMethod(
	namespace,
	clientIP,
	method string,
	variables map[string]string,
) ([]error, error) {
	return uc.repository.ExecuteMethod(uc.ctx, shared.Namespace(namespace), method, variables, clientIP)
}

func (uc *ArrowUsecases) List() (map[string]arrow.Arrow, []error, error) {
	return uc.repository.List(uc.ctx)
}

func (uc *ArrowUsecases) Get(namespace string) (*arrow.Arrow, []error, error) {
	arrow, warns, err := uc.repository.Get(uc.ctx, shared.Namespace(namespace))

	return &arrow, warns, err
}

func (uc *ArrowUsecases) StopMethod(namespace, method string) ([]error, error) {
	return uc.repository.StopMethod(uc.ctx, shared.Namespace(namespace), method)
}

func (uc *ArrowUsecases) KillMethod(namespace, method string) ([]error, error) {
	return uc.repository.StopMethod(uc.ctx, shared.Namespace(namespace), method)
}

func (uc *ArrowUsecases) ListenChannel() {
	// TODO: Add implementation logic
}
