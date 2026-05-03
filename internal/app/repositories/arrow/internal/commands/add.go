package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

type AddArrow struct {
	Namespace           domain.Namespace
	ArrowMeta           domain.ArrowMeta
	Variables           []domain.Variable
	Netbridge           []netbridge.PortDef
	Targets             map[domain.OS]domain.Target
	DirectInstall       bool
	InstalledConstraint string
}

func (c AddArrow) AggregateID() string {
	return c.Namespace.String()
}

func (c AddArrow) EventName() string {
	return "arrow.added." + c.Namespace.String()
}

func (c AddArrow) ShouldSnapshot() bool {
	return true
}

func (c AddArrow) Validate(
	current *domain.Arrow,
) error {
	if current != nil {
		return fmt.Errorf("add arrow: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c AddArrow) EmitEvent(
	_ *domain.Arrow,
) domain.Arrow {
	return domain.Arrow{
		Namespace:           c.Namespace,
		ArrowMeta:           c.ArrowMeta,
		Variables:           c.Variables,
		Netbridge:           c.Netbridge,
		Targets:             c.Targets,
		UserInstalled:       c.DirectInstall,
		InstalledConstraint: c.InstalledConstraint,
	}
}
