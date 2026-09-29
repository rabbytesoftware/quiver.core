package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// RecordAvailable stamps what the last drift check found ahead of an existing
// row; a nil Available marks the row current.
type RecordAvailable struct {
	Namespace domain.Namespace
	Available *domain.Available
}

func (c RecordAvailable) AggregateID() string {
	return c.Namespace.String()
}

func (c RecordAvailable) EventName() string {
	return "arrow.available_checked." + c.Namespace.String()
}

func (c RecordAvailable) ShouldSnapshot() bool {
	return true
}

func (c RecordAvailable) Validate(
	current *domain.Arrow,
) error {
	if current == nil {
		return fmt.Errorf("record available: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RecordAvailable) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	next := *current
	next.Available = c.Available
	return next
}
