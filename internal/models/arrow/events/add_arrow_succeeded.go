package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
)

// ArrowAddArrowSucceeded records that an arrow was successfully added to the system.
type ArrowAddArrowSucceeded struct {
	domain.BaseEvent
	Namespace string       `json:"namespace"`
	Arrow     *arrow.Arrow `json:"arrow"`
}

func NewArrowAddArrowSucceeded(
	namespace string,
	arr *arrow.Arrow,
) domain.Event {
	return &ArrowAddArrowSucceeded{
		BaseEvent: domain.NewBaseEvent(
			namespace,
			"arrow",
			ArrowAddArrowSucceededType,
		),
		Namespace: namespace,
		Arrow:     arr,
	}
}

func (e *ArrowAddArrowSucceeded) GetEventType() string {
	return ArrowAddArrowSucceededType
}

func (e *ArrowAddArrowSucceeded) ShouldCheckIdempotency() bool {
	return false
}

