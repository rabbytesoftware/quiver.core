package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// SetChannel changes which release channel an already-catalogued arrow
// tracks. Used both by the self-update path (stamping quiver.core's own
// catalog row from a config preference) and, in future, by an API caller
// changing an already-installed arrow's channel.
type SetChannel struct {
	Namespace domain.Namespace
	Channel   string
}

func (c SetChannel) AggregateID() string {
	return c.Namespace.String()
}

func (c SetChannel) EventName() string {
	return "arrow.channel_set." + c.Namespace.String()
}

func (c SetChannel) ShouldSnapshot() bool {
	return true
}

// Validate requires the aggregate to already exist: setting a channel
// answers "what should this already-known arrow track", which is
// meaningless for a namespace nothing has ever added.
func (c SetChannel) Validate(
	current *domain.Arrow,
) error {
	if current == nil {
		return fmt.Errorf("set channel: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c SetChannel) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	next := *current
	next.Channel = c.Channel
	return next
}
