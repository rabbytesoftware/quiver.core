package events

import (
	"fmt"

	"github.com/go-playground/validator/v10"
	v1 "github.com/rabbytesoftware/quiver/internal/async/v1"
	"github.com/rabbytesoftware/quiver/internal/core/es/event"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
)

type AddSucceededEvent interface {
	WithArrowNamespace(
		arrowNamespace string,
	) AddSucceededEvent
	WithArrow(
		arrow *arrow.Arrow,
	) AddSucceededEvent
	WithStep(
		step int64,
	) AddSucceededEvent

	Build() (*event.Event[addSucceededEvent], error)
}

type addSucceededEvent struct {
	ArrowNamespace string       `json:"arrow_namespace" validate:"required"`
	Arrow          *arrow.Arrow `json:"arrow" validate:"required"`
	Step           int64        `json:"step" validate:"required"`
}

func NewAddSucceededEvent() AddSucceededEvent {
	return &addSucceededEvent{}
}

func (e *addSucceededEvent) WithArrowNamespace(
	arrowNamespace string,
) AddSucceededEvent {
	e.ArrowNamespace = arrowNamespace
	return e
}

func (e *addSucceededEvent) WithArrow(
	arr *arrow.Arrow,
) AddSucceededEvent {
	e.Arrow = arr
	return e
}

func (e *addSucceededEvent) WithStep(
	step int64,
) AddSucceededEvent {
	e.Step = step
	return e
}

func (e *addSucceededEvent) Build() (*event.Event[addSucceededEvent], error) {
	validate := validator.New()
	if err := validate.Struct(e); err != nil {
		return nil, fmt.Errorf("invalid arrow add succeeded event: %w", err)
	}

	return event.NewEvent[addSucceededEvent]().
		WithAggregateID(e.ArrowNamespace).
		WithAggregateType(ArrowAggregateType).
		WithAggregateVersion(e.Step).
		WithEventType(ArrowAddArrowSucceededType).
		WithEventVersion(v1.AsyncV1Namespace).
		WithPayload(e).
		Build()
}
