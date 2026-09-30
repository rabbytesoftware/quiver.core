package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
)

// AdvanceArrow moves an existing row to a newly installed ref in place: the
// row's identity, install stamps and selector kind are kept, while the
// manifest and Resolved are replaced, and Available is cleared. With
// KeepNewerAvailable, Available is cleared only when it names what was just
// installed: an update's commit keeps a release a check recorded beyond its
// target while the update ran.
type AdvanceArrow struct {
	Namespace          domain.Namespace
	ArrowMeta          domain.ArrowMeta
	Variables          []domain.Variable
	Netbridge          []netbridge.PortDef
	Targets            map[domain.OS]domain.Target
	Readme             string
	Resolved           domain.Resolved
	KeepNewerAvailable bool
}

func (c AdvanceArrow) AggregateID() string {
	return c.Namespace.String()
}

func (c AdvanceArrow) EventName() string {
	return "arrow.advanced." + c.Namespace.String()
}

func (c AdvanceArrow) ShouldSnapshot() bool {
	return true
}

func (c AdvanceArrow) Validate(
	current *domain.Arrow,
) error {
	if current == nil {
		return fmt.Errorf("advance arrow: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c AdvanceArrow) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	next := *current
	next.ArrowMeta = c.ArrowMeta
	next.Variables = c.Variables
	next.Netbridge = c.Netbridge
	next.Targets = c.Targets
	next.Readme = c.Readme
	next.Resolved = c.Resolved
	if !c.KeepNewerAvailable || c.names(next.Available) {
		next.Available = nil
	}
	return next
}

func (c AdvanceArrow) names(
	available *domain.Available,
) bool {
	return available == nil || (available.Ref == c.Resolved.Ref && available.Commit == c.Resolved.Commit)
}
