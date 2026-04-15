package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type RemoveQuiver struct {
	Namespace domain.Namespace
}

func (c RemoveQuiver) AggregateID() string {
	return c.Namespace.String()
}

func (c RemoveQuiver) EventName() string {
	return "quiver.removed"
}

func (c RemoveQuiver) ShouldSnapshot() bool {
	return true
}

func (c RemoveQuiver) Validate(current *domain.Quiver) error {
	if current == nil {
		return fmt.Errorf("remove quiver: %w", asynxModels.ErrValidation)
	}
	if current.Removed {
		return fmt.Errorf("remove quiver: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RemoveQuiver) EmitEvent(current *domain.Quiver) domain.Quiver {
	return domain.Quiver{
		Namespace: current.Namespace,
		Manifest:  current.Manifest,
		Removed:   true,
	}
}
