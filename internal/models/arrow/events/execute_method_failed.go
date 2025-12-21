package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

// ArrowExecuteMethodFailed records that a method execution failed.
type ArrowExecuteMethodFailed struct {
	domain.BaseEvent
	Namespace   string `json:"namespace"`
	Method      string `json:"method"`
	ExecutionID string `json:"execution_id"`
	CurrentStep int    `json:"current_step"`
	TotalSteps  int    `json:"total_steps"`
	Error       struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewArrowExecuteMethodFailed(
	namespace, method, executionID string,
	currentStep, totalSteps int,
	errorCode, errorMessage string,
) domain.Event {
	return &ArrowExecuteMethodFailed{
		BaseEvent: domain.NewBaseEvent(
			namespace,
			"arrow",
			ArrowExecuteMethodFailedType,
		),
		Namespace:   namespace,
		Method:      method,
		ExecutionID: executionID,
		CurrentStep: currentStep,
		TotalSteps:  totalSteps,
		Error: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{
			Code:    errorCode,
			Message: errorMessage,
		},
	}
}

func (e *ArrowExecuteMethodFailed) GetEventType() string {
	return ArrowExecuteMethodFailedType
}

func (e *ArrowExecuteMethodFailed) ShouldCheckIdempotency() bool {
	return false
}

