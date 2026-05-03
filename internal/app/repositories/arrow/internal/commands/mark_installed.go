package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

// MarkInstalled stamps InstalledAt and InstalledRef on an existing Arrow aggregate.
// Called from the post-execution hook after _install lifecycle succeeds.
type MarkInstalled struct {
	Namespace    domain.Namespace
	InstalledAt  time.Time
	InstalledRef string
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
	updated.InstalledRef = c.InstalledRef
	return updated
}
