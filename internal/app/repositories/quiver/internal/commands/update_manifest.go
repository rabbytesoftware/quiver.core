package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type UpdateQuiverManifest struct {
	Namespace domain.Namespace
}

func (c UpdateQuiverManifest) AggregateID() string {
	return c.Namespace.String()
}

func (c UpdateQuiverManifest) EventName() string {
	return "quiver.updated"
}

func (c UpdateQuiverManifest) ShouldSnapshot() bool {
	return true
}

func (c UpdateQuiverManifest) Validate(current *domain.Quiver) error {
	if current == nil {
		return fmt.Errorf("update quiver: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c UpdateQuiverManifest) EmitEvent(current *domain.Quiver) domain.Quiver {
	return domain.Quiver{
		Namespace:  current.Namespace,
		FollowedAt: current.FollowedAt,
	}
}
