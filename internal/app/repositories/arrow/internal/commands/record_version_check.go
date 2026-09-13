package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// RecordVersionCheck stamps the outcome of a passive version-drift check onto
// an already-catalogued arrow. It is sent only when the freshly computed
// outcome differs from what the aggregate already carries — a check that
// reconfirms the existing answer sends nothing.
type RecordVersionCheck struct {
	Namespace      domain.Namespace
	Outdated       bool
	RecommendedRef string
}

func (c RecordVersionCheck) AggregateID() string {
	return c.Namespace.String()
}

func (c RecordVersionCheck) EventName() string {
	return "arrow.version_checked." + c.Namespace.String()
}

func (c RecordVersionCheck) ShouldSnapshot() bool {
	return true
}

// Validate requires the aggregate to already exist: a version check answers
// "is the arrow I already know about behind", which is meaningless for a
// namespace nothing has ever added.
func (c RecordVersionCheck) Validate(
	current *domain.Arrow,
) error {
	if current == nil {
		return fmt.Errorf("record version check: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RecordVersionCheck) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	next := *current
	next.Outdated = c.Outdated
	next.RecommendedRef = c.RecommendedRef
	return next
}
