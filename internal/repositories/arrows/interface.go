package arrows

import (
	"context"

	domain "github.com/rabbytesoftware/quiver/internal/models/arrow"
)

type ArrowState struct {
	Status   string
	Action   *ActionState
	Metadata *domain.Arrow
}

type ActionState struct {
	Method     string
	Title      map[string]string
	StepIndex  int
	Steps      int
}

type ArrowsInterface interface {
	AddArrow(
		ctx context.Context, 
		namespace, path string, 
		force bool, 
		idempotencyKey, clientIP string,
	) (domain.Arrow, []error, error)
	DeleteArrow(
		ctx context.Context, 
		namespace string, 
		force bool, 
		idempotencyKey, clientIP string,
	) ([]error, error)
	ExecuteMethod(
		ctx context.Context, 
		namespace, method string, 
		variables map[string]string, 
		idempotencyKey, clientIP string,
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
