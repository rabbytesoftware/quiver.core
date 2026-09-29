package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// RecordAvailable stamps what the last drift check found ahead of an existing
// row; a nil Available marks the row current. JudgedResolved is the Resolved
// the check judged: an answer about a row that has since moved is refused.
type RecordAvailable struct {
	Namespace      domain.Namespace
	Available      *domain.Available
	JudgedResolved domain.Resolved
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
	if current.Resolved != c.JudgedResolved {
		return fmt.Errorf("record available: judged against a resolved the row has left: %w", asynxModels.ErrValidation)
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
