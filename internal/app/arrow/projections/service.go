package projections

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

// arrowService is the subset of ArrowService consumed by WizardExecutor.
// Defined here to avoid a circular import between projections and arrow packages.
type arrowService interface {
	HasDependents(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (bool, error)
	CleanupAfterUninstall(
		ctx context.Context,
		ns domain.Namespace,
	) error
	BeginExecution(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		userVars map[string]string,
	) error
	GetWorkDir(
		ctx context.Context,
		ns domain.Namespace,
	) (string, error)
}
