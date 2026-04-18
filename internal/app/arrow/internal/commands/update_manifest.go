package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type UpdateArrowManifest struct {
	Namespace domain.Namespace
	Manifest  domain.ArrowManifest
}

func (c UpdateArrowManifest) AggregateID() string {
	return c.Namespace.String()
}

func (c UpdateArrowManifest) EventName() string {
	return "arrow.updated"
}

func (c UpdateArrowManifest) ShouldSnapshot() bool {
	return true
}

func (c UpdateArrowManifest) Validate(current *domain.Arrow) error {
	if current == nil {
		return fmt.Errorf("update arrow: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c UpdateArrowManifest) EmitEvent(current *domain.Arrow) domain.Arrow {
	versions := current.Versions
	if versions == nil {
		versions = make(map[string]domain.ArrowVersion)
	}
	av := domain.ArrowVersion{
		ArrowMeta: c.Manifest.ArrowMeta,
		Variables: c.Manifest.Variables,
		Netbridge: c.Manifest.Netbridge,
		Targets:   c.Manifest.Targets,
		InstalledAt: c.Manifest.InstalledAt,
		DirectInstall: c.Manifest.UserInstalled,
	}
	versions[c.Manifest.Version] = av
	return domain.Arrow{
		Namespace: current.Namespace,
		Versions:  versions,
	}
}
