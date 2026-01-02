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
	rp  arrows.ArrowsInterface
	ctx context.Context
}

func NewApiArrowsUsecases(repos *repositories.Repositories) *ApiArrowUsescases {

	return &ApiArrowUsescases{
		rp:  repos.GetArrows(),
		ctx: context.Background(),
	}
}

func (uc *ApiArrowUsescases) Add(searchValue, valueType, clientIP string) (*arrow.Arrow, []error, error) {
	var arrow arrow.Arrow
	var addErrors []error
	var addError error

	switch valueType {
	case "url":
		arr, errs, err := uc.rp.Add(uc.ctx, "", searchValue, false, clientIP)
		arrow = arr
		addErrors = errs
		addError = err
	case "directory":
		arr, errs, err := uc.rp.Add(uc.ctx, "", searchValue, false, clientIP)
		arrow = arr
		addErrors = errs
		addError = err
	case "namespace":
		arr, errs, err := uc.rp.Add(uc.ctx, shared.Namespace(searchValue), "", false, clientIP)
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
