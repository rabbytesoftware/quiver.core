package events

import (
	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

const (
	ArrowExecuteMethodRequestedType = "arrow.ExecuteMethod.Requested"
)

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
	aggregateVersion int64,
	correlationID string,
	parentID *string,
	metadata map[string]interface{},
) domain.Event {
	executionID := uuid.New().String()
	return &ArrowExecuteMethodRequested{
		BaseEvent: domain.NewBaseEventWithMetadata(
			namespace,
			"arrow",
			ArrowExecuteMethodRequestedType,
			aggregateVersion,
			correlationID,
			parentID,
			metadata,
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

