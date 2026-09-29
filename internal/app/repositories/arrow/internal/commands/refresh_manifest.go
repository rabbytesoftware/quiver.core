package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
)

// RefreshManifest replaces an existing row's manifest with its update
// target's, leaving Resolved and Available alone: the update that runs the
// new manifest's steps has not committed yet.
type RefreshManifest struct {
	Namespace domain.Namespace
	ArrowMeta domain.ArrowMeta
	Variables []domain.Variable
	Netbridge []netbridge.PortDef
	Targets   map[domain.OS]domain.Target
	Readme    string
}

func (c RefreshManifest) AggregateID() string {
	return c.Namespace.String()
}

func (c RefreshManifest) EventName() string {
	return "arrow.manifest_refreshed." + c.Namespace.String()
}

func (c RefreshManifest) ShouldSnapshot() bool {
	return true
}

func (c RefreshManifest) Validate(
	current *domain.Arrow,
) error {
	if current == nil {
		return fmt.Errorf("refresh manifest: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RefreshManifest) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	next := *current
	next.ArrowMeta = c.ArrowMeta
	next.Variables = c.Variables
	next.Netbridge = c.Netbridge
	next.Targets = c.Targets
	next.Readme = c.Readme
	return next
}
