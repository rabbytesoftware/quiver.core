package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

// ArrowExecuteMethodSucceeded records that a method execution completed successfully.
type ArrowExecuteMethodSucceeded struct {
	domain.BaseEvent
	Namespace   string                 `json:"namespace"`
	Method      string                 `json:"method"`
	ExecutionID string                 `json:"execution_id"`
	Result      map[string]interface{} `json:"result"`
}

func NewArrowExecuteMethodSucceeded(
	namespace, method, executionID string,
	result map[string]interface{},
) domain.Event {
	return &ArrowExecuteMethodSucceeded{
		BaseEvent: domain.NewBaseEvent(
			namespace,
			"arrow",
			ArrowExecuteMethodSucceededType,
		),
		Namespace:   namespace,
		Method:      method,
		ExecutionID: executionID,
		Result:      result,
	}
}

func (e *ArrowExecuteMethodSucceeded) GetEventType() string {
	return ArrowExecuteMethodSucceededType
}

func (e *ArrowExecuteMethodSucceeded) ShouldCheckIdempotency() bool {
	return false
}

