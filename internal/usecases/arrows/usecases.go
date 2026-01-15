package arrows

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/shared"
	"github.com/rabbytesoftware/quiver/internal/repositories"
	"github.com/rabbytesoftware/quiver/internal/repositories/arrows"
)

type ApiArrowUsecases struct {
	repository arrows.ArrowsInterface
	ctx        context.Context
}

func NewApiArrowsUsecases(repos *repositories.Repositories) *ApiArrowUsecases {

	return &ApiArrowUsecases{
		repository: repos.GetArrows(),
		ctx:        context.Background(),
	}
}

func (uc *ApiArrowUsecases) Add(
	searchValue,
	valueType,
	clientIP string,
	force bool,
) (arrow.Arrow, []error, error) {
	return uc.repository.Add(uc.ctx, searchValue, force, clientIP)
}

func (uc *ApiArrowUsecases) Remove(namespace, clientIP string) ([]error, error) {
	return  uc.repository.Remove(uc.ctx, shared.Namespace(namespace), false, clientIP)
}

func (uc *ApiArrowUsecases) ExecuteMethod(
	namespace,
	clientIP,
	method string,
	variables map[string]string,
) ([]error, error) {
	return  uc.repository.ExecuteMethod(uc.ctx, shared.Namespace(namespace), method, variables, clientIP)
}

func (uc *ApiArrowUsecases) List() (map[string]arrow.Arrow, []error, error) {
	return uc.repository.List(uc.ctx)
}

func (uc *ApiArrowUsecases) Get(namespace string) (*arrow.Arrow, []error, error) {
	arrow, warns, err := uc.repository.Get(uc.ctx, shared.Namespace(namespace))

	return &arrow, warns, err
}

func (uc *ApiArrowUsecases) StopMethod(namespace, method string) ([]error, error) {
	return uc.repository.StopMethod(uc.ctx, shared.Namespace(namespace), method)
}

func (uc *ApiArrowUsecases) KillMethod(namespace, method string) ([]error, error) {
	return uc.repository.StopMethod(uc.ctx, shared.Namespace(namespace), method)
}

func (uc *ApiArrowUsecases) ListenChannel() {
	// TODO: Add implementation logic
}
