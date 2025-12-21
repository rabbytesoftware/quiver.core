package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
)

const (
	ArrowAggregateType  = "Arrow"
	ArrowAddedEventType = "wizard.arrow.added"
)

type ArrowAdded struct {
	domain.BaseEvent
	ArrowID string       `json:"arrow_id"`
	Arrow   *arrow.Arrow `json:"arrow"`
	Source  string       `json:"source"`
	AddedBy string       `json:"added_by"`
}

func NewArrowAdded(
	arrowID string,
	arr *arrow.Arrow,
	source,
	addedBy string,
) domain.Event {
	return &ArrowAdded{
		BaseEvent: domain.NewBaseEvent(
			arrowID,
			ArrowAggregateType,
			ArrowAddedEventType,
		),
		ArrowID: arrowID,
		Arrow:   arr,
		Source:  source,
		AddedBy: addedBy,
	}
}

func (e *ArrowAdded) GetEventType() string {
	return ArrowAddedEventType
}
