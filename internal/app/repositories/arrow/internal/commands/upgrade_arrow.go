package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

type UpgradeArrow struct {
	Namespace           domain.Namespace
	OldNamespace        domain.Namespace
	ArrowMeta           domain.ArrowMeta
	Variables           []domain.Variable
	Netbridge           []netbridge.PortDef
	Targets             map[domain.OS]domain.Target
	InstalledConstraint string
}

func (c UpgradeArrow) AggregateID() string {
	return c.Namespace.String()
}

func (c UpgradeArrow) EventName() string {
	return "arrow.upgraded." + c.Namespace.String()
}

func (c UpgradeArrow) ShouldSnapshot() bool {
	return true
}

func (c UpgradeArrow) Validate(current *domain.Arrow) error {
	if current != nil {
		return fmt.Errorf("upgrade arrow: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c UpgradeArrow) EmitEvent(_ *domain.Arrow) domain.Arrow {
	return domain.Arrow{
		Namespace:           c.Namespace,
		ArrowMeta:           c.ArrowMeta,
		Variables:           c.Variables,
		Netbridge:           c.Netbridge,
		Targets:             c.Targets,
		InstalledConstraint: c.InstalledConstraint,
		UpgradedFromNs:      c.OldNamespace,
	}
}
