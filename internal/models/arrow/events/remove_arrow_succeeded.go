package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

// ArrowRemoveArrowSucceeded records that an arrow was successfully removed from the system.
type ArrowRemoveArrowSucceeded struct {
	domain.BaseEvent
	Namespace string `json:"namespace"`
}

func NewArrowRemoveArrowSucceeded(
	namespace string,
) domain.Event {
	return &ArrowRemoveArrowSucceeded{
		BaseEvent: domain.NewBaseEvent(
			namespace,
			"arrow",
			ArrowRemoveArrowSucceededType,
		),
		Namespace: namespace,
	}
}

func (e *ArrowRemoveArrowSucceeded) GetEventType() string {
	return ArrowRemoveArrowSucceededType
}

func (e *ArrowRemoveArrowSucceeded) ShouldCheckIdempotency() bool {
	return false
}

