package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// requireCurrentExecution rejects progress reported for a run the arrow is no
// longer performing. A nil execution means the run already ended; a different
// id means a stop or an update took the arrow over, and the earlier run's
// remaining steps belong to nothing.
func requireCurrentExecution(
	op string,
	current *domainRuntime.ArrowRuntime,
	executionID string,
) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("%s: %w", op, asynxModels.ErrValidation)
	}
	if current.Execution == nil {
		return fmt.Errorf("%s: %w", op, asynxModels.ErrValidation)
	}
	if current.Execution.ID != executionID {
		return fmt.Errorf("%s: %w: %w", op, apperrors.ErrExecutionSuperseded, asynxModels.ErrValidation)
	}
	return nil
}
