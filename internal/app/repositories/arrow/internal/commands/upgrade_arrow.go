package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
)

type UpgradeArrow struct {
	Namespace           domain.Namespace
	OldNamespace        domain.Namespace
	ArrowMeta           domain.ArrowMeta
	Variables           []domain.Variable
	Netbridge           []netbridge.PortDef
	Targets             map[domain.OS]domain.Target
	Readme              string
	InstalledConstraint string
	// Channel carries the old row's tracked channel onto the new one. Set
	// directly here, in the same event as the ref swap, rather than via a
	// follow-up SetChannel call: SetChannel's own EmitEvent clears
	// InstalledConstraint unconditionally (an explicit channel switch
	// supersedes it), which is wrong for this case -- an ordinary upgrade
	// carrying an unchanged channel forward must leave InstalledConstraint
	// untouched.
	Channel string
	// AlreadyReady carries through to the new Arrow unchanged; see its doc
	// comment on domain.Arrow.
	AlreadyReady bool
	// UserInstalled carries the old row's flag onto the new one, so an arrow
	// the user explicitly installed does not silently lose that fact on its
	// first upgrade.
	UserInstalled bool
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
		Readme:              c.Readme,
		InstalledConstraint: c.InstalledConstraint,
		Channel:             c.Channel,
		UpgradedFromNs:      c.OldNamespace,
		AlreadyReady:        c.AlreadyReady,
		UserInstalled:       c.UserInstalled,
	}
}
