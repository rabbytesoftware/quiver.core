package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

const (
	ArrowAddArrowRequestedType = "arrow.AddArrow.Requested"
)

type ArrowAddArrowRequested struct {
	domain.BaseEvent
	Namespace string `json:"namespace"`
	Path      string `json:"path"`
	ForceAdd  bool   `json:"force_add"`
}

func NewArrowAddArrowRequested(
	namespace, path string,
	force bool,
	aggregateVersion int64,
	correlationID string,
	parentID *string,
	metadata map[string]interface{},
) domain.Event {
	return &ArrowAddArrowRequested{
		BaseEvent: domain.NewBaseEventWithMetadata(
			namespace,
			"arrow",
			ArrowAddArrowRequestedType,
			aggregateVersion,
			correlationID,
			parentID,
			metadata,
		),
		Namespace: namespace,
		Path:      path,
		ForceAdd:  force,
	}
}

func (e *ArrowAddArrowRequested) GetEventType() string {
	return ArrowAddArrowRequestedType
}

