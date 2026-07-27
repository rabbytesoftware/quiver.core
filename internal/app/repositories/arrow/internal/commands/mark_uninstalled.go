package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// MarkUninstalled is MarkInstalled's inverse: it clears the InstalledAt stamp,
// so an arrow whose _uninstall lifecycle has run stops claiming its ref is on
// disk. Called from the post-execution hook.
//
// It clears that field and nothing else. InstalledConstraint is written when the
// namespace is added, not when it is installed, and an uninstalled arrow still
// resolves updates through it; UserInstalled records the intent to keep the
// arrow in the catalog, which an uninstall does not revoke.
type MarkUninstalled struct {
	Namespace domain.Namespace
}

func (c MarkUninstalled) AggregateID() string {
	return c.Namespace.String()
}

func (c MarkUninstalled) EventName() string {
	return "arrow.uninstalled." + c.Namespace.String()
}

func (c MarkUninstalled) ShouldSnapshot() bool {
	return true
}

func (c MarkUninstalled) Validate(
	current *domain.Arrow,
) error {
	if current == nil {
		return fmt.Errorf("mark uninstalled: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c MarkUninstalled) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	updated := *current
	updated.InstalledAt = time.Time{}
	return updated
}
