package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type SetUserInstalled struct {
	Namespace domain.Namespace
}

func (c SetUserInstalled) AggregateID() string {
	return c.Namespace.String()
}

func (c SetUserInstalled) EventName() string {
	return "arrow.user_installed"
}

func (c SetUserInstalled) ShouldSnapshot() bool {
	return true
}

func (c SetUserInstalled) Validate(
	current *domain.Arrow,
) error {
	if current == nil {
		return fmt.Errorf("set user installed: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetUserInstalled) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	updated := *current
	updated.UserInstalled = true
	return updated
}
