package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type FollowQuiver struct {
	Quiver domain.Quiver
}

func (c FollowQuiver) AggregateID() string  { return c.Quiver.Namespace.String() }
func (c FollowQuiver) EventName() string    { return "quiver.followed" }
func (c FollowQuiver) ShouldSnapshot() bool { return true }

func (c FollowQuiver) Validate(current *domain.Quiver) error {
	if current != nil {
		return fmt.Errorf("follow quiver: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c FollowQuiver) EmitEvent(_ *domain.Quiver) domain.Quiver {
	q := c.Quiver
	q.FollowedAt = time.Now()
	return q
}
