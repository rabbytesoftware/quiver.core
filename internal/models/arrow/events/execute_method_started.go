package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

// ArrowExecuteMethodStarted records that a method execution has started.
type ArrowExecuteMethodStarted struct {
	domain.BaseEvent
	Namespace   string `json:"namespace"`
	Method      string `json:"method"`
	ExecutionID string `json:"execution_id"`
}

func NewArrowExecuteMethodStarted(
	namespace, method, executionID string,
) domain.Event {
	return &ArrowExecuteMethodStarted{
		BaseEvent: domain.NewBaseEvent(
			namespace,
			"arrow",
			ArrowExecuteMethodStartedType,
		),
		Namespace:   namespace,
		Method:      method,
		ExecutionID: executionID,
	}
}

func (e *ArrowExecuteMethodStarted) GetEventType() string {
	return ArrowExecuteMethodStartedType
}

func (e *ArrowExecuteMethodStarted) ShouldCheckIdempotency() bool {
	return false
}

