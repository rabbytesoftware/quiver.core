package arrows

import (
	"context"

	eventsourcing "github.com/rabbytesoftware/quiver/internal/core/es"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/rabbytesoftware/quiver/internal/models/shared"
)

type ArrowsInterface interface {
	Get(
		ctx context.Context,
		namespace shared.Namespace,
	) (arrow.Arrow, []error, error)
	List(
		ctx context.Context,
	) (map[string]arrow.Arrow, []error, error)

	Add(
		ctx context.Context,
		namespace shared.Namespace,
		path string,
		force bool,
		clientIP string,
	) (arrow.Arrow, []error, error)
	Remove(
		ctx context.Context,
		namespace shared.Namespace,
		force bool,
		clientIP string,
	) ([]error, error)

	ExecuteMethod(
		ctx context.Context,
		namespace shared.Namespace,
		method string,
		variables map[string]string,
		clientIP string,
	) ([]error, error)
	StopMethod(
		ctx context.Context,
		namespace shared.Namespace,
		method string,
	) ([]error, error)
}

type ArrowsRepository struct {
	es *eventsourcing.EventSourcing
}

func NewArrowsRepository() ArrowsInterface {
	return &ArrowsRepository{
		es: nil,
	}
}

func (r *ArrowsRepository) Get(
	ctx context.Context,
	namespace shared.Namespace,
) (arrow.Arrow, []error, error) {
	return arrow.Arrow{}, nil, nil
}

func (r *ArrowsRepository) List(
	ctx context.Context,
) (map[string]arrow.Arrow, []error, error) {
	return nil, nil, nil
}

func (r *ArrowsRepository) Add(
	ctx context.Context,
	namespace shared.Namespace,
	path string,
	force bool,
	clientIP string,
) (arrow.Arrow, []error, error) {
	return arrow.Arrow{}, nil, nil
}

func (r *ArrowsRepository) Remove(
	ctx context.Context,
	namespace shared.Namespace,
	force bool,
	clientIP string,
) ([]error, error) {
	return nil, nil
}

func (r *ArrowsRepository) ExecuteMethod(
	ctx context.Context,
	namespace shared.Namespace,
	method string,
	variables map[string]string,
	clientIP string,
) ([]error, error) {
	return nil, nil
}

func (r *ArrowsRepository) StopMethod(
	ctx context.Context,
	namespace shared.Namespace,
	method string,
) ([]error, error) {
	return nil, nil
}
