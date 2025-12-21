package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

// ArrowAddArrowFailed records that an arrow addition failed.
type ArrowAddArrowFailed struct {
	domain.BaseEvent
	Namespace string `json:"namespace"`
	Error     struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewArrowAddArrowFailed(
	namespace string,
	errorCode, errorMessage string,
) domain.Event {
	return &ArrowAddArrowFailed{
		BaseEvent: domain.NewBaseEvent(
			namespace,
			"arrow",
			ArrowAddArrowFailedType,
		),
		Namespace: namespace,
		Error: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{
			Code:    errorCode,
			Message: errorMessage,
		},
	}
}

func (e *ArrowAddArrowFailed) GetEventType() string {
	return ArrowAddArrowFailedType
}

func (e *ArrowAddArrowFailed) ShouldCheckIdempotency() bool {
	return false
}

