package manifold

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/versioning"
)

type (
	// Host answers the host-specific questions manifest resolution runs into:
	// where a raw file lives and which refs a repository defaults to.
	Host = hosts.Host

	// HostLookup resolves the host serving a namespace, reporting false when
	// none does.
	HostLookup = hosts.Lookup
)

var (
	// ErrUnknownSelector reports a selector or ref a snapshot does not hold.
	ErrUnknownSelector = versioning.ErrUnknownSelector

	// ErrNotAdmitted reports a ref its selector could never resolve to.
	ErrNotAdmitted = versioning.ErrNotAdmitted
)

// ChannelsOf buckets a snapshot's tags into channels.
func ChannelsOf(
	snap domain.RefSnapshot,
) []ChannelInfo {
	return versioning.ChannelsOf(snap)
}

// ClassifySelector decides what a selector follows in snap.
func ClassifySelector(
	selector string,
	snap domain.RefSnapshot,
) (domain.SelectorKind, error) {
	return versioning.ClassifySelector(selector, snap)
}

// DefaultChannel names the channel a refless install follows.
func DefaultChannel(
	snap domain.RefSnapshot,
) (string, error) {
	return versioning.DefaultChannel(snap)
}

// RefCommit reports the commit ref names right now for a row of kind following selector.
func RefCommit(
	kind domain.SelectorKind,
	selector string,
	ref string,
	snap domain.RefSnapshot,
) (string, bool) {
	return versioning.RefCommit(kind, selector, ref, snap)
}

// HasEmptyComponent reports whether selector has an empty ref component.
func HasEmptyComponent(
	selector string,
) bool {
	return versioning.HasEmptyComponent(selector)
}

// Target resolves what a selector of the given kind points at in snap.
func Target(
	kind domain.SelectorKind,
	selector string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	return versioning.Target(kind, selector, snap)
}

// Drift reports whether a row following selector is behind snap, and its target.
func Drift(
	kind domain.SelectorKind,
	selector string,
	resolved domain.Resolved,
	snap domain.RefSnapshot,
) (domain.Available, bool, error) {
	return versioning.Drift(kind, selector, resolved, snap)
}

// Admit resolves ref as state declared for a row following selector.
func Admit(
	kind domain.SelectorKind,
	selector string,
	ref string,
	snap domain.RefSnapshot,
) (domain.Available, error) {
	return versioning.Admit(kind, selector, ref, snap)
}
