package arrow

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/models/arrow"
)

type ArrowViewInt interface {
	Get(
		ctx context.Context,
		namespace string,
	) (arrow.Arrow, error)
	List(
		ctx context.Context,
	) (map[string]arrow.Arrow, error)

	Add(
		ctx context.Context,
		arrow arrow.Arrow,
	) error
	Update(
		ctx context.Context,
		arrow arrow.Arrow,
	) error
	Remove(
		ctx context.Context,
		arrow arrow.Arrow,
	) error
}

type ArrowViewImpl struct {
}

func NewArrowView() ArrowViewInt {
	return &ArrowViewImpl{}
}

func (v *ArrowViewImpl) Get(
	ctx context.Context,
	namespace string,
) (arrow.Arrow, error) {
	return arrow.Arrow{}, nil
}

func (v *ArrowViewImpl) List(
	ctx context.Context,
) (map[string]arrow.Arrow, error) {
	return nil, nil
}

func (v *ArrowViewImpl) Add(
	ctx context.Context,
	arrow arrow.Arrow,
) error {

	return nil
}

func (v *ArrowViewImpl) Update(
	ctx context.Context,
	arrow arrow.Arrow,
) error {
	return nil
}

func (v *ArrowViewImpl) Remove(
	ctx context.Context,
	arrow arrow.Arrow,
) error {
	return nil
}
