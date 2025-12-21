package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

// ArrowAddArrowRequested records that a user has requested to add a new arrow.
type ArrowAddArrowRequested struct {
	domain.BaseEvent
	Namespace string `json:"namespace"`
	Path      string `json:"path"`
	ForceAdd  bool   `json:"force_add"`
}

func NewArrowAddArrowRequested(
	namespace, path string,
	force bool,
) domain.Event {
	return &ArrowAddArrowRequested{
		BaseEvent: domain.NewBaseEvent(
			namespace,
			"arrow",
			ArrowAddArrowRequestedType,
		),
		Namespace: namespace,
		Path:      path,
		ForceAdd:  force,
	}
}

func (e *ArrowAddArrowRequested) GetEventType() string {
	return ArrowAddArrowRequestedType
}

func (e *ArrowAddArrowRequested) ShouldCheckIdempotency() bool {
	return true
}

