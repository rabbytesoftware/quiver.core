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
	return false
}

func (c UpdateArrowManifest) Validate(current *domain.Arrow) error {
	if current == nil {
		return fmt.Errorf("update arrow: %w", asynxModels.ErrValidation)
	}
	if current.Removed {
		return fmt.Errorf("update arrow: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c UpdateArrowManifest) EmitEvent(current *domain.Arrow) domain.Arrow {
	return domain.Arrow{
		Namespace: current.Namespace,
		Manifest:  c.Manifest,
		Removed:   false,
	}
}
