package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type RemoveArrow struct {
	Namespace domain.Namespace
}

func (c RemoveArrow) AggregateID() string {
	return c.Namespace.String()
}

func (c RemoveArrow) EventName() string {
	return "arrow.removed"
}

func (c RemoveArrow) ShouldSnapshot() bool {
	return false
}

func (c RemoveArrow) Validate(current *domain.Arrow) error {
	if current == nil {
		return fmt.Errorf("remove arrow: %w", asynxModels.ErrValidation)
	}
	if current.Removed {
		return fmt.Errorf("remove arrow: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RemoveArrow) EmitEvent(current *domain.Arrow) domain.Arrow {
	return domain.Arrow{
		Namespace: current.Namespace,
		Manifest:  current.Manifest,
		Removed:   true,
	}
}
