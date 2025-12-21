package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

// ArrowExecuteMethodProgressUpdated records progress updates during method execution.
type ArrowExecuteMethodProgressUpdated struct {
	domain.BaseEvent
	Namespace   string `json:"namespace"`
	Method      string `json:"method"`
	ExecutionID string `json:"execution_id"`
	CurrentStep int    `json:"current_step"`
	TotalSteps  int    `json:"total_steps"`
	Message     string `json:"message,omitempty"`
}

func NewArrowExecuteMethodProgressUpdated(
	namespace, method, executionID string,
	currentStep, totalSteps int,
	message string,
) domain.Event {
	return &ArrowExecuteMethodProgressUpdated{
		BaseEvent: domain.NewBaseEvent(
			namespace,
			"arrow",
			ArrowExecuteMethodProgressUpdatedType,
		),
		Namespace:   namespace,
		Method:      method,
		ExecutionID: executionID,
		CurrentStep: currentStep,
		TotalSteps:  totalSteps,
		Message:     message,
	}
}

func (e *ArrowExecuteMethodProgressUpdated) GetEventType() string {
	return ArrowExecuteMethodProgressUpdatedType
}

func (e *ArrowExecuteMethodProgressUpdated) ShouldCheckIdempotency() bool {
	return false
}

