package events

import (
	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

// ArrowExecuteMethodRequested records that a user has requested to execute a method on an arrow.
type ArrowExecuteMethodRequested struct {
	domain.BaseEvent
	Namespace   string            `json:"namespace"`
	Method      string            `json:"method"`
	ExecutionID string            `json:"execution_id"`
	Variables   map[string]string `json:"variables"`
}

func NewArrowExecuteMethodRequested(
	namespace, method string,
	variables map[string]string,
) domain.Event {
	executionID := uuid.New().String()
	return &ArrowExecuteMethodRequested{
		BaseEvent: domain.NewBaseEvent(
			namespace,
			"arrow",
			ArrowExecuteMethodRequestedType,
		),
		Namespace:   namespace,
		Method:      method,
		ExecutionID: executionID,
		Variables:   variables,
	}
}

func (e *ArrowExecuteMethodRequested) GetEventType() string {
	return ArrowExecuteMethodRequestedType
}

func (e *ArrowExecuteMethodRequested) ShouldCheckIdempotency() bool {
	return true
}

