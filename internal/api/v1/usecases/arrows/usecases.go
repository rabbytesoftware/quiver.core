package arrows

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/shared"
	"github.com/rabbytesoftware/quiver/internal/repositories"
	"github.com/rabbytesoftware/quiver/internal/repositories/arrows"
)

type ApiArrowUsescases struct {
	repository  arrows.ArrowsInterface
	ctx context.Context
}

func NewApiArrowsUsecases(repos *repositories.Repositories) *ApiArrowUsescases {

	return &ApiArrowUsescases{
		repository:  repos.GetArrows(),
		ctx: context.Background(),
	}
}

func (uc *ApiArrowUsescases) Add(searchValue, valueType, clientIP string) (*arrow.Arrow, []error, error) {
	var arrow arrow.Arrow
	var addErrors []error
	var addError error

	switch valueType {
	case "url":
		arr, errs, err := uc.repository.Add(uc.ctx, "", searchValue, false, clientIP)
		arrow = arr
		addErrors = errs
		addError = err
	case "directory":
		arr, errs, err := uc.repository.Add(uc.ctx, "", searchValue, false, clientIP)
		arrow = arr
		addErrors = errs
		addError = err
	case "namespace":
		arr, errs, err := uc.repository.Add(uc.ctx, shared.Namespace(searchValue), "", false, clientIP)
		arrow = arr
		addErrors = errs
		addError = err
	default:
		return nil, nil, fmt.Errorf("invalid value type: %s expected: namespace, directory or url", valueType)
	}

	if addError != nil {
		return nil, nil, addError
	}

	if len(addErrors) > 0 {
		return nil, addErrors, nil
	}

	return &arrow, nil, nil
}

func (uc *ApiArrowUsescases) Remove(namespace, clientIP string) ([]error, error) {
	errs, err := uc.repository.Remove(uc.ctx, shared.Namespace(namespace), false, clientIP)

	// TODO: Implement better error messages
	if err != nil {
		return nil, err
	}

	if len(errs) > 0 {
		return errs, nil
	}

	return nil, nil
}

func (uc *ApiArrowUsescases) ExecuteMethod(namespace, clientIP, method string,variables map[string]string) ([]error, error) {
	errs, err := uc.repository.ExecuteMethod(uc.ctx, shared.Namespace(namespace), method,variables, clientIP)

	// TODO: Implement better error messages
	if err != nil {
		return nil, err
	}

	if len(errs) > 0 {
		return errs, nil
	}

	return nil, nil
}

func (uc *ApiArrowUsescases) List() (map[string]arrow.Arrow, []error, error) {
	arrows, errs, err := uc.repository.List(uc.ctx)

	if err != nil {
		return nil, nil, err
	}

	if len(errs) > 0 {
		return nil, errs, nil
	}

	return arrows, nil, nil
}

func (uc *ApiArrowUsescases) Get(namespace string) (*arrow.Arrow, []error, error) {
	arrow, errs, err := uc.repository.Get(uc.ctx, shared.Namespace(namespace))

	if err != nil {
		return nil, nil, err
	}

	if len(errs) > 0 {
		return nil, errs, nil
	}

	return &arrow, nil, nil
}