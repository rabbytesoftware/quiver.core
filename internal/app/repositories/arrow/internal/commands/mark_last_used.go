package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// MarkLastUsed stamps LastUsedAt on an existing Arrow aggregate. Called from
// the post-execution hook after the _execute lifecycle succeeds.
type MarkLastUsed struct {
	Namespace  domain.Namespace
	LastUsedAt time.Time
}

func (c MarkLastUsed) AggregateID() string {
	return c.Namespace.String()
}

func (c MarkLastUsed) EventName() string {
	return "arrow.last_used." + c.Namespace.String()
}

func (c MarkLastUsed) ShouldSnapshot() bool {
	return true
}

func (c MarkLastUsed) Validate(
	current *domain.Arrow,
) error {
	if current == nil {
		return fmt.Errorf("mark last used: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c MarkLastUsed) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	updated := *current
	updated.LastUsedAt = c.LastUsedAt
	return updated
}
