package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// MarkInstalled stamps InstalledAt on an existing Arrow aggregate. Called from
// the post-execution hook after the _install lifecycle succeeds.
//
// The stamp names no ref because it cannot name a different one: the aggregate
// is keyed by namespace@ref and an _install can only have put that ref on disk.
type MarkInstalled struct {
	Namespace   domain.Namespace
	InstalledAt time.Time
}

func (c MarkInstalled) AggregateID() string {
	return c.Namespace.String()
}

func (c MarkInstalled) EventName() string {
	return "arrow.installed." + c.Namespace.String()
}

func (c MarkInstalled) ShouldSnapshot() bool {
	return true
}

func (c MarkInstalled) Validate(
	current *domain.Arrow,
) error {
	if current == nil {
		return fmt.Errorf("mark installed: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c MarkInstalled) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	updated := *current
	updated.InstalledAt = c.InstalledAt
	return updated
}
