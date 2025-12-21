package arrows

import (
	"context"

	domain "github.com/rabbytesoftware/quiver/internal/models/arrow"
)

type ArrowState struct {
	Status   string        `json:"status"`
	Action   *ActionState  `json:"action"`
	Metadata *domain.Arrow `json:"metadata"`
}

type ActionState struct {
	Method    string            `json:"method"`
	Title     map[string]string `json:"title"`
	StepIndex int               `json:"step_index"`
	Steps     int               `json:"steps"`
}

type ArrowsInterface interface {
	AddArrow(
		ctx context.Context,
		namespace, path string,
		force bool,
		clientIP string,
	) (domain.Arrow, []error, error)
	DeleteArrow(
		ctx context.Context,
		namespace string,
		force bool,
		clientIP string,
	) ([]error, error)
	ExecuteMethod(
		ctx context.Context,
		namespace, method string,
		variables map[string]string,
		clientIP string,
	) ([]error, error)
	StopMethod(
		ctx context.Context,
		namespace, method string,
	) ([]error, error)
	GetArrow(
		ctx context.Context,
		namespace string,
	) (domain.Arrow, []error, error)
	ListArrows(
		ctx context.Context,
	) (map[string]ArrowState, []error, error)
}
