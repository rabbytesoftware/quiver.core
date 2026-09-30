package versioning

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/versioning/internal/admit"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/versioning/internal/drift"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/versioning/internal/selector"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/versioning/internal/snapshot"
)

type (
	// RefLister reads every ref of a repository in one round trip.
	RefLister = snapshot.RefLister

	// Snapshots reads a repository's refs, cached for a TTL.
	Snapshots = snapshot.Snapshots
)

// New builds Snapshots that read refs and reuse a result for ttl by clock.
func New(
	refs RefLister,
	clock func() time.Time,
	ttl time.Duration,
) Snapshots {
	return snapshot.New(refs, clock, ttl)
}

// LatestStable names the latest tag of snap's stable channel.
func LatestStable(
	snap domain.RefSnapshot,
) (string, bool) {
	return selector.LatestStable(snap)
}

// DefaultBranch names snap's HEAD branch and the commit it is at.
func DefaultBranch(
	snap domain.RefSnapshot,
) (branch, commit string, ok bool) {
	return selector.DefaultBranch(snap)
}

// ChannelsOf buckets a snapshot's tags into channels.
func ChannelsOf(
	snap domain.RefSnapshot,
) []models.ChannelInfo {
	return selector.ChannelsOf(snap)
}

// ClassifySelector decides what a selector follows in snap.
func ClassifySelector(
	sel string,
	snap domain.RefSnapshot,
) (domain.SelectorKind, error) {
	return selector.ClassifySelector(sel, snap)
}

// DefaultChannel names the channel a refless install follows.
func DefaultChannel(
	snap domain.RefSnapshot,
) (string, error) {
	return selector.DefaultChannel(snap)
}

// RefCommit reports the commit ref names right now for a row of kind following sel.
func RefCommit(
	kind domain.SelectorKind,
	sel string,
	ref string,
	snap domain.RefSnapshot,
) (string, bool) {
	return selector.RefCommit(kind, sel, ref, snap)
}

// HasEmptyComponent reports whether sel has an empty ref component.
func HasEmptyComponent(
	sel string,
) bool {
	return selector.HasEmptyComponent(sel)
}

// Target resolves what a selector of the given kind points at in snap.
func Target(
	kind domain.SelectorKind,
	sel string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	return drift.Target(kind, sel, snap)
}

// Drift reports whether a row following sel is behind snap, and its target.
func Drift(
	kind domain.SelectorKind,
	sel string,
	resolved domain.Resolved,
	snap domain.RefSnapshot,
) (domain.Available, bool, error) {
	return drift.Drift(kind, sel, resolved, snap)
}

// Admit resolves ref as state declared for a row following sel.
func Admit(
	kind domain.SelectorKind,
	sel string,
	ref string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	return admit.Admit(kind, sel, ref, snap)
}
