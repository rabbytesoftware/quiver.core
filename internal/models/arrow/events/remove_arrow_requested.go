package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

// ArrowRemoveArrowRequested records that a user has requested to remove an existing arrow.
type ArrowRemoveArrowRequested struct {
	domain.BaseEvent
	Namespace string `json:"namespace"`
	Force     bool   `json:"force"`
}

func NewArrowRemoveArrowRequested(
	namespace string,
	force bool,
) domain.Event {
	return &ArrowRemoveArrowRequested{
		BaseEvent: domain.NewBaseEvent(
			namespace,
			"arrow",
			ArrowRemoveArrowRequestedType,
		),
		Namespace: namespace,
		Force:     force,
	}
}

func (e *ArrowRemoveArrowRequested) GetEventType() string {
	return ArrowRemoveArrowRequestedType
}

func (e *ArrowRemoveArrowRequested) ShouldCheckIdempotency() bool {
	return true
}

