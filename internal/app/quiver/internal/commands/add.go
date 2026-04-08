package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type AddQuiver struct {
	Namespace domain.Namespace
	Manifest  domain.QuiverManifest
}

func (c AddQuiver) AggregateID() string {
	return c.Namespace.String()
}

func (c AddQuiver) EventName() string {
	return "quiver.added"
}

func (c AddQuiver) ShouldSnapshot() bool {
	return false
}

func (c AddQuiver) Validate(current *domain.Quiver) error {
	if current != nil && !current.Removed {
		return fmt.Errorf("add quiver: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c AddQuiver) EmitEvent(_ *domain.Quiver) domain.Quiver {
	return domain.Quiver{
		Namespace: c.Namespace,
		Manifest:  c.Manifest,
		Removed:   false,
	}
}
