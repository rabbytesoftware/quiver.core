package events

import (
	"fmt"

	"github.com/go-playground/validator/v10"
	v1 "github.com/rabbytesoftware/quiver/internal/async/v1"
	"github.com/rabbytesoftware/quiver/internal/core/es/event"
)

type AddFailedEvent interface {
	WithArrowNamespace(
		arrowNamespace string,
	) AddFailedEvent
	WithErrorCode(
		errorCode string,
	) AddFailedEvent
	WithErrorMessage(
		errorMessage string,
	) AddFailedEvent
	WithStep(
		step int64,
	) AddFailedEvent

	Build() (*event.Event[addFailedEvent], error)
}

type addFailedEvent struct {
	ArrowNamespace string `json:"arrow_namespace" validate:"required" gorm:"column:arrow_namespace;index"`
	Error          struct {
		Code    string `json:"code" validate:"required" gorm:"column:code"`
		Message string `json:"message" validate:"required" gorm:"column:message"`
	} `json:"error"`
	Step int64 `json:"step" validate:"required" gorm:"column:step"`
}

func NewAddFailedEvent() AddFailedEvent {
	return &addFailedEvent{}
}

func (e *addFailedEvent) WithArrowNamespace(
	arrowNamespace string,
) AddFailedEvent {
	e.ArrowNamespace = arrowNamespace
	return e
}

func (e *addFailedEvent) WithErrorCode(
	errorCode string,
) AddFailedEvent {
	e.Error.Code = errorCode
	return e
}

func (e *addFailedEvent) WithErrorMessage(
	errorMessage string,
) AddFailedEvent {
	e.Error.Message = errorMessage
	return e
}

func (e *addFailedEvent) WithStep(
	step int64,
) AddFailedEvent {
	e.Step = step
	return e
}

func (e *addFailedEvent) Build() (*event.Event[addFailedEvent], error) {
	validate := validator.New()
	if err := validate.Struct(e); err != nil {
		return nil, fmt.Errorf("invalid arrow add failed event: %w", err)
	}

	return event.NewEvent[addFailedEvent]().
		WithAggregateID(e.ArrowNamespace).
		WithAggregateType(ArrowAggregateType).
		WithAggregateVersion(e.Step).
		WithEventType(ArrowAddArrowFailedType).
		WithEventVersion(v1.AsyncV1Namespace).
		WithPayload(e).
		Build()
}
