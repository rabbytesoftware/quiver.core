package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
)

type UpdateArrowManifest struct {
	Namespace domain.Namespace
	ArrowMeta domain.ArrowMeta
	Variables []domain.Variable
	Netbridge []netbridge.PortDef
	Targets   map[domain.OS]domain.Target
}

func (c UpdateArrowManifest) AggregateID() string {
	return c.Namespace.String()
}

func (c UpdateArrowManifest) EventName() string {
	return "arrow.updated." + c.Namespace.String()
}

func (c UpdateArrowManifest) ShouldSnapshot() bool {
	return true
}

func (c UpdateArrowManifest) Validate(
	current *domain.Arrow,
) error {
	if current == nil {
		return fmt.Errorf("update arrow: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c UpdateArrowManifest) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	updated := *current
	updated.ArrowMeta = c.ArrowMeta
	updated.Variables = c.Variables
	updated.Netbridge = c.Netbridge
	updated.Targets = c.Targets
	return updated
}
