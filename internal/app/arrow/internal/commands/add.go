package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type AddArrow struct {
	Namespace domain.Namespace
	Manifest  domain.ArrowManifest
}

func (c AddArrow) AggregateID() string {
	return c.Namespace.String()
}

func (c AddArrow) EventName() string {
	return "arrow.added"
}

func (c AddArrow) ShouldSnapshot() bool {
	return false
}

func (c AddArrow) Validate(current *domain.Arrow) error {
	if current != nil {
		return fmt.Errorf("add arrow: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c AddArrow) EmitEvent(_ *domain.Arrow) domain.Arrow {
	return domain.Arrow{
		Namespace: c.Namespace,
		Manifest:  c.Manifest,
	}
}
