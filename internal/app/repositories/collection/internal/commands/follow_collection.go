package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type FollowCollection struct {
	Collection domain.Collection
}

func (c FollowCollection) AggregateID() string  { return c.Collection.Namespace.String() }
func (c FollowCollection) EventName() string    { return "collection.followed" }
func (c FollowCollection) ShouldSnapshot() bool { return true }

func (c FollowCollection) Validate(current *domain.Collection) error {
	if current != nil {
		return fmt.Errorf("follow collection: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c FollowCollection) EmitEvent(_ *domain.Collection) domain.Collection {
	q := c.Collection
	q.FollowedAt = time.Now()
	return q
}
