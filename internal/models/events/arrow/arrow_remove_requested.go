package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

const (
	ArrowRemoveArrowRequestedType = "arrow.RemoveArrow.Requested"
)

type ArrowRemoveArrowRequested struct {
	domain.BaseEvent
	Namespace string `json:"namespace"`
	Force     bool   `json:"force"`
}

func NewArrowRemoveArrowRequested(
	namespace string,
	force bool,
	aggregateVersion int64,
	correlationID string,
	parentID *string,
	metadata map[string]interface{},
) domain.Event {
	return &ArrowRemoveArrowRequested{
		BaseEvent: domain.NewBaseEventWithMetadata(
			namespace,
			"arrow",
			ArrowRemoveArrowRequestedType,
			aggregateVersion,
			correlationID,
			parentID,
			metadata,
		),
		Namespace: namespace,
		Force:     force,
	}
}

func (e *ArrowRemoveArrowRequested) GetEventType() string {
	return ArrowRemoveArrowRequestedType
}

