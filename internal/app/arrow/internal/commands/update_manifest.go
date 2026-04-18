package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type UpdateArrowManifest struct {
	Namespace domain.Namespace
	Version   domain.ArrowVersion
}

func (c UpdateArrowManifest) AggregateID() string {
	return c.Namespace.BareNamespace().String()
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

	key := c.Namespace.Ref()
	if key == "" {
		key = domain.VersionLatestRef
	}

	av := c.Version
	av.InstalledRef = c.Namespace.Ref()
	versions[key] = av

	return domain.Arrow{
		Namespace: current.Namespace,
		Versions:  versions,
	}
}
