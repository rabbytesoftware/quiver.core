package events

import (
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

// ArrowRemoveArrowFailed records that an arrow removal failed.
type ArrowRemoveArrowFailed struct {
	domain.BaseEvent
	Namespace string `json:"namespace"`
	Error     struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewArrowRemoveArrowFailed(
	namespace string,
	errorCode, errorMessage string,
) domain.Event {
	return &ArrowRemoveArrowFailed{
		BaseEvent: domain.NewBaseEvent(
			namespace,
			"arrow",
			ArrowRemoveArrowFailedType,
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

func (e *ArrowRemoveArrowFailed) GetEventType() string {
	return ArrowRemoveArrowFailedType
}

func (e *ArrowRemoveArrowFailed) ShouldCheckIdempotency() bool {
	return false
}

