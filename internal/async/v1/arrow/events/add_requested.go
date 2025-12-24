package events

import (
	"fmt"

	"github.com/go-playground/validator/v10"
	v1 "github.com/rabbytesoftware/quiver/internal/async/v1"
	"github.com/rabbytesoftware/quiver/internal/core/es/event"
)

type AddRequestedEventInt interface {
	WithArrowNamespace(
		arrowNamespace string,
	) AddRequestedEventInt
	WithPath(
		path string,
	) AddRequestedEventInt
	WithForceAdd(
		forceAdd bool,
	) AddRequestedEventInt
	WithStep(
		step int64,
	) AddRequestedEventInt

	Build() (*event.Event[AddRequestedEvent], error)
}

type AddRequestedEvent struct {
	ArrowNamespace string `json:"arrow_namespace" validate:"required"`
	Path           string `json:"path" validate:"required"`
	ForceAdd       bool   `json:"force_add"`
	Step           int64  `json:"step" validate:"required"`
}

func NewAddRequestedEvent() AddRequestedEventInt {
	return &AddRequestedEvent{}
}

func (e *AddRequestedEvent) WithArrowNamespace(
	arrowNamespace string,
) AddRequestedEventInt {
	e.ArrowNamespace = arrowNamespace
	return e
}

func (e *AddRequestedEvent) WithPath(
	path string,
) AddRequestedEventInt {
	e.Path = path
	return e
}

func (e *AddRequestedEvent) WithForceAdd(
	forceAdd bool,
) AddRequestedEventInt {
	e.ForceAdd = forceAdd
	return e
}

func (e *AddRequestedEvent) WithStep(
	step int64,
) AddRequestedEventInt {
	e.Step = step
	return e
}

func (e *AddRequestedEvent) Build() (*event.Event[AddRequestedEvent], error) {
	validate := validator.New()
	if err := validate.Struct(e); err != nil {
		return nil, fmt.Errorf("invalid arrow add requested event: %w", err)
	}

	return event.NewEvent[AddRequestedEvent]().
		WithAggregateID(e.ArrowNamespace).
		WithAggregateType(ArrowAggregateType).
		WithAggregateVersion(e.Step).
		WithEventType(ArrowAddArrowRequestedType).
		WithEventVersion(v1.AsyncV1Namespace).
		WithPayload(e).
		Build()
}
