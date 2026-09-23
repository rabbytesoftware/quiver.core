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

// EmitEvent stamps the new channel and clears everything that answered a
// question this switch just superseded.
//
// InstalledConstraint is cleared unconditionally: a glob install
// (resolveGlob) leaves both a constraint AND a classified Channel on the
// same row, and ResolveTrackedRef is constraint-first — left in place, a
// later channel switch would have zero effect on drift-check resolution,
// the exact "dropdown does nothing" bug this command exists to prevent, in
// a narrower case. An explicit, user-driven channel switch expresses clear
// intent to track that channel from now on; it supersedes whatever
// constraint an earlier install left behind. Clearing it is a no-op for
// every other caller (self-registration's stampConfiguredChannel stamps
// rows that never had a constraint to begin with).
//
// Outdated and RecommendedRef are cleared for the same reason: they are
// answers to "is the OLD channel/constraint behind", and RecordVersionCheck
// is their only other writer. Left stale, a previous channel's
// RecommendedRef could survive indefinitely (runVersionCheck writes nothing
// at all when a check errors, e.g. offline), and since upgradeRef prefers
// RecommendedRef outright when set, a click on Update right after switching
// channels could upgrade onto the OLD channel's target instead of the new
// one. A freshly switched channel starts clean, as "not yet checked", until
// CheckVersionNow's own check lands a real answer.
func (c SetChannel) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	next := *current
	next.Channel = c.Channel
	next.InstalledConstraint = ""
	next.Outdated = false
	next.RecommendedRef = ""
	return next
}
