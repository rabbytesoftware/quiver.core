package arrows

import (
	"context"
	"fmt"

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

func (uc *ApiArrowUsecases) Add(searchValue, valueType, clientIP string) (*arrow.Arrow, []error, error) {
	var arrow arrow.Arrow
	var addWarnings []error
	var addError error

	switch valueType {
	case "url":
		arr, warns, err := uc.repository.Add(uc.ctx, "", searchValue, false, clientIP)
		arrow = arr
		addWarnings = warns
		addError = err
	case "directory":
		arr, warns, err := uc.repository.Add(uc.ctx, "", searchValue, false, clientIP)
		arrow = arr
		addWarnings = warns
		addError = err
	case "namespace":
		arr, warns, err := uc.repository.Add(uc.ctx, shared.Namespace(searchValue), "", false, clientIP)
		arrow = arr
		addWarnings = warns
		addError = err
	default:
		return nil, nil, fmt.Errorf("invalid value type: %s expected: namespace, directory or url", valueType)
	}

	if addError != nil {
		return nil, nil, addError
	}

	return &arrow, addWarnings, nil
}

func (uc *ApiArrowUsecases) Remove(namespace, clientIP string) ([]error, error) {
	warns, err := uc.repository.Remove(uc.ctx, shared.Namespace(namespace), false, clientIP)

	// TODO: Implement better error messages
	if err != nil {
		return nil, err
	}

	return warns, nil
}

func (uc *ApiArrowUsecases) ExecuteMethod(namespace, clientIP, method string, variables map[string]string) ([]error, error) {
	warns, err := uc.repository.ExecuteMethod(uc.ctx, shared.Namespace(namespace), method, variables, clientIP)

	// TODO: Implement better error messages
	if err != nil {
		return nil, err
	}

	return warns, nil
}

func (uc *ApiArrowUsecases) List() (map[string]arrow.Arrow, []error, error) {
	arrows, warns, err := uc.repository.List(uc.ctx)

	if err != nil {
		return nil, nil, err
	}

	return arrows, warns, nil
}

func (uc *ApiArrowUsecases) Get(namespace string) (*arrow.Arrow, []error, error) {
	arrow, warns, err := uc.repository.Get(uc.ctx, shared.Namespace(namespace))

	if err != nil {
		return nil, nil, err
	}

	return &arrow, warns, nil
}

func (uc *ApiArrowUsecases) StopMethod(namespace, method string) ([]error, error) {
	warns, err := uc.repository.StopMethod(uc.ctx, shared.Namespace(namespace), method)

	if err != nil {
		return nil, err
	}

	return warns, nil
}

func (uc *ApiArrowUsecases) KillMethod(namespace, method string) ([]error, error) {
	warns, err := uc.repository.StopMethod(uc.ctx, shared.Namespace(namespace), method)

	if err != nil {
		return nil, err
	}

	return warns, nil
}

func (uc *ApiArrowUsecases) ListenChannel() {
	// TODO: Add implementation logic
}
