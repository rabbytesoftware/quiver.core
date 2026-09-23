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
//
// Ref, when set, pins the arrow to that exact ref within Channel rather than
// the channel's own latest — see domain.Arrow.PinnedRef. Empty clears any
// previously pinned ref, tracking the channel's latest again.
type SetChannel struct {
	Namespace domain.Namespace
	Channel   string
	Ref       string
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
// question this switch just superseded. This is deliberately the ONLY
// place a channel is ever recorded: upgradeRef (usecases/arrow.go) carries
// a channel forward through UpgradeArrow's own event instead of a
// follow-up call to this command, specifically because the clears below
// would be wrong for that case — see UpgradeArrow's own Channel field doc
// comment. The two remaining callers, both of which this command's clears
// suit correctly, are switchChannel (an explicit, user-driven switch) and
// selfarrow.go's stampConfiguredChannel (self-registration).
//
// InstalledConstraint is cleared unconditionally: a glob install
// (resolveGlob) leaves both a constraint AND a classified Channel on the
// same row, and ResolveTrackedRef is constraint-first — left in place, a
// later channel switch would have zero effect on drift-check resolution,
// the exact "dropdown does nothing" bug this command exists to prevent, in
// a narrower case. An explicit, user-driven channel switch expresses clear
// intent to track that channel from now on; it supersedes whatever
// constraint an earlier install left behind. Clearing it is a no-op for
// stampConfiguredChannel's self-arrow row specifically: self-registration
// never goes through a glob install, so that row never carries a
// constraint to begin with.
//
// Outdated and RecommendedRef are cleared for the same reason: they are
// answers to "is the OLD channel/constraint behind", and RecordVersionCheck
// is their only other writer. Left stale, a previous channel's
// RecommendedRef could survive indefinitely (runVersionCheck writes nothing
// at all when a check errors, e.g. offline), and since upgradeRef prefers
// RecommendedRef outright when set, a click on Update right after switching
// channels could upgrade onto the OLD channel's target instead of the new
// one. A freshly switched channel starts clean, as "not yet checked", until
// the next check lands a real answer.
//
// Unlike InstalledConstraint, this clear is NOT a no-op for
// stampConfiguredChannel: it re-stamps the self-arrow's channel on every
// boot when self_update_channel is configured, so it wipes that row's
// Outdated/RecommendedRef on every boot too. This is bounded and
// self-healing rather than a bug: the fields default back to empty either
// way, the next passive TTL check (or a boot-time CheckVersionNow, if one
// is ever wired to selfarrow's path) lands a fresh answer, and
// runVersionCheck syncs the runtime state badge unconditionally before its
// own early-return, so the visible badge never lags behind reality for
// long.
func (c SetChannel) EmitEvent(
	current *domain.Arrow,
) domain.Arrow {
	next := *current
	next.Channel = c.Channel
	next.PinnedRef = c.Ref
	next.InstalledConstraint = ""
	next.Outdated = false
	next.RecommendedRef = ""
	return next
}
